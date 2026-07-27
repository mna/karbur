package acctmw

import (
	"net/http"
	"testing"

	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/pgxadapt"
	"codeberg.org/mna/karbur/pgdb/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogout(t *testing.T) {
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

			// using the /load action so that the session is loaded before hitting the logout middleware
			dh := &deferHandler{}
			accts, srv := setupAccounts(t, pool, map[Action]http.Handler{ActionLoad: dh})
			dh.h = accts.Logout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				acct := acctctx.Account(r.Context())
				ssnID := acctctx.SessionID(r.Context())
				assert.Nil(t, acct)
				assert.Empty(t, ssnID)
				w.WriteHeader(http.StatusOK)
			}))
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

			// request the page without login
			res, err := client.Get(srv.URL + "/load")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)
			for _, ck := range res.Cookies() {
				if ck.Name == "__Host-ssn" {
					require.Less(t, ck.MaxAge, 0)
					require.Empty(t, ck.Value)
				}
			}

			// do a successful login
			doLoginWithClient(t, client, srv.URL, "a@b", "123")

			// request the page after a login
			res, err = client.Get(srv.URL + "/load")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)
			for _, ck := range res.Cookies() {
				if ck.Name == "__Host-ssn" {
					require.Less(t, ck.MaxAge, 0)
					require.Empty(t, ck.Value)
				}
			}
		})
	}
}
