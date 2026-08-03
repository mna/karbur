package acctmw

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/pgxadapt"
	"codeberg.org/mna/karbur/pgdb/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelete(t *testing.T) {
	cases := []struct {
		name  string
		setup func() pgdb.Pool
	}{
		{"pgx", func() pgdb.Pool { db := testdb.NewPgx(t, "", ""); return pgxadapt.ToPool(db) }},
		// {"sql", func() pgdb.Pool { db := testdb.NewSQL(t, "", ""); return sqladapt.ToPool(db) }},
		// {"pq", func() pgdb.Pool { db := testdb.NewPqSQL(t, "", ""); return sqladapt.ToPool(db) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := tc.setup()

			accts, srv := setupAccounts(t, pool, map[Action]http.Handler{
				ActionDelete: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					acct := acctctx.Account(r.Context())
					ssnID := acctctx.SessionID(r.Context())
					assert.NotNil(t, acct)
					assert.Empty(t, ssnID)
					w.WriteHeader(http.StatusOK)
				}),
			})
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

			// request the "delete" page without login
			res, err := client.Get(srv.URL + "/delete?password=123")
			require.NoError(t, err)
			require.Equal(t, http.StatusForbidden, res.StatusCode)

			doLoginWithClient(t, client, srv.URL, "a@b", "123")

			// request the "delete" page without a password
			res, err = client.Get(srv.URL + "/delete")
			require.NoError(t, err)
			require.Equal(t, http.StatusBadRequest, res.StatusCode)

			// request the "delete" page with wrong password
			res, err = client.Get(srv.URL + "/delete?password=456")
			require.NoError(t, err)
			require.Equal(t, http.StatusBadRequest, res.StatusCode)

			// session cookie is still present
			assertSessionCookiePresent(t, client.Jar, srv.URL)

			// request the "delete" page with correct password
			res, err = client.Get(srv.URL + "/delete?password=123")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)

			// session cookie is now absent
			assertSessionCookieAbsent(t, client.Jar, srv.URL)

			// login now fails, unknown account
			res, err = client.Post(srv.URL+"/login", "application/json",
				strings.NewReader(fmt.Sprintf(`{"email":%q, "password":%q}`, "a@b", "123")))
			require.NoError(t, err)
			require.Equal(t, http.StatusBadRequest, res.StatusCode)
		})
	}
}
