package acctmw

import (
	"testing"

	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/pgxadapt"
	"codeberg.org/mna/karbur/pgdb/testdb"
)

func TestAnonymous(t *testing.T) {
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
			_ = pool

			// var expectLoggedIn bool
			// var accountID uuid.UUID
			// var sessionID string
			// accts, srv := setupAccounts(t, pool, map[Action]http.Handler{ActionLoad: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 	acct := acctctx.Account(r.Context())
			// 	ssnID := acctctx.SessionID(r.Context())
			// 	if expectLoggedIn {
			// 		assert.NotNil(t, acct)
			// 		assert.Equal(t, "a@b", acct.Email)
			// 		assert.NotEmpty(t, ssnID)
			// 		accountID = acct.ID
			// 		sessionID = ssnID
			// 	} else {
			// 		assert.Nil(t, acct)
			// 		assert.Empty(t, ssnID)
			// 	}
			// 	w.WriteHeader(http.StatusOK)
			// })})
			// accts.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			// 	code := errors.Code(err)
			// 	if code == 0 {
			// 		code = http.StatusInternalServerError
			// 	}
			// 	t.Log(err)
			// 	w.WriteHeader(code)
			// }
		})
	}
}
