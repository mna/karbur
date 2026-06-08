package acctmw

import (
	"net/http"
	"testing"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/pgxadapt"
	"codeberg.org/mna/karbur/pgdb/testdb"
	"github.com/stretchr/testify/require"
)

func TestAuthorizeDeny(t *testing.T) {
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

			dh := &deferHandler{}
			accts, srv := setupAccounts(t, pool, map[Action]http.Handler{ActionAuthorize: dh})
			accts.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
				code := errors.Code(err)
				if code == 0 {
					code = http.StatusInternalServerError
				}
				t.Log(err)
				w.WriteHeader(code)
			}

			// create clients for anonymous, authenticated and verified accounts
			anonClient := newBrowserClient(t)
			authnClient := newBrowserClient(t)
			verifClient := newBrowserClient(t)

			// create a valid account for "a@b" and log it in
			createAccountWithClient(t, authnClient, srv.URL, "a@b", "123")
			doLoginWithClient(t, authnClient, srv.URL, "a@b", "123")
			acctAuthn, err := accounts.ByEmail(ctx, pool, "a@b")
			require.NoError(t, err)

			// create a valid account for "b@c" and log it in, verify it
			createAccountWithClient(t, verifClient, srv.URL, "b@c", "456")
			doLoginWithClient(t, verifClient, srv.URL, "b@c", "456")
			_, err = pool.Exec(ctx, `UPDATE "accounts_accounts" SET verified = now() WHERE email = $1`, "b@c")
			require.NoError(t, err)
			acctVerif, err := accounts.ByEmail(ctx, pool, "b@c")
			require.NoError(t, err)

			// create some groups
			err = accounts.CreateGroups(ctx, pool, []string{"admin", "user", "other"})
			require.NoError(t, err)

			// acctAuthn is user and other
			err = accounts.SetMembership(ctx, pool, acctAuthn.ID, []string{"user", "other"})
			require.NoError(t, err)
			// acctVerif is admin and user
			err = accounts.SetMembership(ctx, pool, acctVerif.ID, []string{"user", "admin"})
			require.NoError(t, err)

			cases := []struct {
				desc   string
				client *http.Client
				allow  []string
				deny   []string
				want   int
			}{
				{
					desc:   "anyone allowed, anon",
					client: anonClient,
					allow:  []string{AccessAnyone},
					want:   http.StatusOK,
				},
				{
					desc:   "anyone allowed, authn",
					client: authnClient,
					allow:  []string{AccessAnyone},
					want:   http.StatusOK,
				},
				{
					desc:   "anyone allowed, verif",
					client: verifClient,
					allow:  []string{AccessAnyone},
					want:   http.StatusOK,
				},
				{
					desc:   "authn allowed, anon",
					client: anonClient,
					allow:  []string{AccessAuthenticated},
					want:   http.StatusForbidden,
				},
				{
					desc:   "authn allowed, authn",
					client: authnClient,
					allow:  []string{AccessAuthenticated},
					want:   http.StatusOK,
				},
				{
					desc:   "authn allowed, verif",
					client: verifClient,
					allow:  []string{AccessAuthenticated},
					want:   http.StatusOK,
				},
				{
					desc:   "verif allowed, anon",
					client: anonClient,
					allow:  []string{AccessVerified},
					want:   http.StatusForbidden,
				},
				{
					desc:   "verif allowed, authn",
					client: authnClient,
					allow:  []string{AccessVerified},
					want:   http.StatusForbidden,
				},
				{
					desc:   "verif allowed, verif",
					client: verifClient,
					allow:  []string{AccessVerified},
					want:   http.StatusOK,
				},

				{
					desc:   "anyone denied, anon",
					client: anonClient,
					deny:   []string{AccessAnyone},
					want:   http.StatusForbidden,
				},
				{
					desc:   "anyone denied, authn",
					client: authnClient,
					deny:   []string{AccessAnyone},
					want:   http.StatusForbidden,
				},
				{
					desc:   "anyone denied, verif",
					client: verifClient,
					deny:   []string{AccessAnyone},
					want:   http.StatusForbidden,
				},
				{
					desc:   "authn denied, anon",
					client: anonClient,
					deny:   []string{AccessAuthenticated},
					want:   http.StatusOK,
				},
				{
					desc:   "authn denied, authn",
					client: authnClient,
					deny:   []string{AccessAuthenticated},
					want:   http.StatusForbidden,
				},
				{
					desc:   "authn denied, verif",
					client: verifClient,
					deny:   []string{AccessAuthenticated},
					want:   http.StatusForbidden,
				},
				{
					desc:   "verif denied, anon",
					client: anonClient,
					deny:   []string{AccessVerified},
					want:   http.StatusOK,
				},
				{
					desc:   "verif denied, authn",
					client: authnClient,
					deny:   []string{AccessVerified},
					want:   http.StatusOK,
				},
				{
					desc:   "verif denied, verif",
					client: verifClient,
					deny:   []string{AccessVerified},
					want:   http.StatusForbidden,
				},

				{
					desc:   "user and other allowed, anon",
					client: anonClient,
					allow:  []string{"user", "other"},
					want:   http.StatusForbidden,
				},
				{
					desc:   "user and other allowed, authn",
					client: authnClient,
					allow:  []string{"user", "other"},
					want:   http.StatusOK,
				},
				{
					desc:   "user and other allowed, verif",
					client: verifClient,
					allow:  []string{"user", "other"},
					want:   http.StatusOK,
				},
				{
					desc:   "user and other denied, anon",
					client: anonClient,
					deny:   []string{"user", "other"},
					want:   http.StatusOK,
				},
				{
					desc:   "user and other denied, authn",
					client: authnClient,
					deny:   []string{"user", "other"},
					want:   http.StatusForbidden,
				},
				{
					desc:   "user and other denied, verif",
					client: verifClient,
					deny:   []string{"user", "other"},
					want:   http.StatusForbidden,
				},

				{
					desc:   "admin allowed, anon",
					client: anonClient,
					allow:  []string{"admin"},
					want:   http.StatusForbidden,
				},
				{
					desc:   "admin allowed, authn",
					client: authnClient,
					allow:  []string{"admin"},
					want:   http.StatusForbidden,
				},
				{
					desc:   "admin allowed, verif",
					client: verifClient,
					allow:  []string{"admin"},
					want:   http.StatusOK,
				},
				{
					desc:   "admin denied, anon",
					client: anonClient,
					deny:   []string{"admin"},
					want:   http.StatusOK,
				},
				{
					desc:   "admin denied, authn",
					client: authnClient,
					deny:   []string{"admin"},
					want:   http.StatusOK,
				},
				{
					desc:   "admin denied, verif",
					client: verifClient,
					deny:   []string{"admin"},
					want:   http.StatusForbidden,
				},

				{
					desc:   "authn allowed, admin denied, anon",
					client: anonClient,
					allow:  []string{AccessAuthenticated},
					deny:   []string{"admin"},
					want:   http.StatusForbidden,
				},
				{
					desc:   "authn allowed, admin denied, authn",
					client: authnClient,
					allow:  []string{AccessAuthenticated},
					deny:   []string{"admin"},
					want:   http.StatusOK,
				},
				{
					desc:   "authn allowed, admin denied, verif",
					client: verifClient,
					allow:  []string{AccessAuthenticated},
					deny:   []string{"admin"},
					want:   http.StatusForbidden,
				},

				{
					desc:   "unknown group allowed, anon",
					client: anonClient,
					allow:  []string{"NOSUCH"},
					want:   http.StatusForbidden,
				},
				{
					desc:   "unknown group allowed, authn",
					client: authnClient,
					allow:  []string{"NOSUCH"},
					want:   http.StatusForbidden,
				},
				{
					desc:   "unknown group allowed, verif",
					client: verifClient,
					allow:  []string{"NOSUCH"},
					want:   http.StatusForbidden,
				},
				{
					desc:   "unknown group denied, anon",
					client: anonClient,
					deny:   []string{"NOSUCH"},
					want:   http.StatusOK,
				},
				{
					desc:   "unknown group denied, authn",
					client: authnClient,
					deny:   []string{"NOSUCH"},
					want:   http.StatusOK,
				},
				{
					desc:   "unknown group denied, verif",
					client: verifClient,
					deny:   []string{"NOSUCH"},
					want:   http.StatusOK,
				},
			}
			for _, c := range cases {
				t.Run(c.desc, func(t *testing.T) {
					dh.h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
					if len(c.allow) > 0 {
						dh.h = accts.Authorize(c.allow)(dh.h)
					}
					if len(c.deny) > 0 {
						dh.h = accts.Deny(c.deny)(dh.h)
					}
					res, err := c.client.Get(srv.URL + "/authorize")
					require.NoError(t, err)
					require.Equal(t, c.want, res.StatusCode)
				})
			}
		})
	}
}
