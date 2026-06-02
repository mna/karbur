package acctmw

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/pgxadapt"
	"codeberg.org/mna/karbur/pgdb/sqladapt"
	"codeberg.org/mna/karbur/pgdb/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	cases := []struct {
		name  string
		setup func() pgdb.Pool
	}{
		{"pgx", func() pgdb.Pool { db := testdb.NewPgx(t, "", ""); return pgxadapt.ToPool(db) }},
		{"sql", func() pgdb.Pool { db := testdb.NewSQL(t, "", ""); return sqladapt.ToPool(db) }},
		{"pq", func() pgdb.Pool { db := testdb.NewPqSQL(t, "", ""); return sqladapt.ToPool(db) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := tc.setup()

			var expectLoggedIn bool
			var accountID int64
			var sessionID string
			accts, srv := setupAccounts(t, pool, map[Action]http.Handler{ActionLoad: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				acct := acctctx.Account(r.Context())
				ssnID := acctctx.SessionID(r.Context())
				if expectLoggedIn {
					assert.NotNil(t, acct)
					assert.Equal(t, "a@b", acct.Email)
					assert.NotEmpty(t, ssnID)
					accountID = acct.ID
					sessionID = ssnID
				} else {
					assert.Nil(t, acct)
					assert.Empty(t, ssnID)
				}
				w.WriteHeader(http.StatusOK)
			})})
			accts.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
				code := errors.Code(err)
				if code == 0 {
					code = http.StatusInternalServerError
				}
				t.Log(err)
				w.WriteHeader(code)
			}

			// use a browser-like client, with a cookie jar
			client := newBrowserClient(t)

			// create a valid account for "a@b"
			createAccountWithClient(t, client, srv.URL, "a@b", "123")

			// request the "load" page without login
			expectLoggedIn = false
			res, err := client.Get(srv.URL + "/load")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)

			// do a failed login
			res, err = client.Post(srv.URL+"/login", "application/json", strings.NewReader(fmt.Sprintf(`{"email":%q, "password":%q}`, "a@b", "456")))
			require.NoError(t, err)
			require.Equal(t, 400, res.StatusCode)

			expectLoggedIn = false
			res, err = client.Get(srv.URL + "/load")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)

			// do a successful login
			doLoginWithClient(t, client, srv.URL, "a@b", "123")

			expectLoggedIn = true
			res, err = client.Get(srv.URL + "/load")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)
			require.NotZero(t, accountID)
			require.NotEmpty(t, sessionID)

			// delete the account, so the session ID does not map to anything
			err = accounts.Delete(t.Context(), pool, accountID)
			require.NoError(t, err)

			expectLoggedIn = false
			res, err = client.Get(srv.URL + "/load")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)

			// delete the session ID (leaving the cookie)
			err = accts.Tokens.Delete(t.Context(), sessionID)
			require.NoError(t, err)

			expectLoggedIn = false
			res, err = client.Get(srv.URL + "/load")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)
		})
	}
}
