package accounts

import (
	"bytes"
	"net/http"
	"net/url"
	"testing"

	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/pgxadapt"
	"codeberg.org/mna/karbur/pgdb/sqladapt"
	"codeberg.org/mna/karbur/pgdb/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogin(t *testing.T) {
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
			accts, srv := setupAccounts(t, pool)

			// create a valid account for "a@b"
			createAccount(t, srv.URL, "a@b", "123")

			cases := []struct {
				desc        string
				contentType string
				body        []byte
				wantCode    int
				wantErr     string
			}{
				{
					desc:        "missing body",
					contentType: "application/json",
					body:        nil,
					wantCode:    http.StatusBadRequest,
					wantErr:     "accounts: email is missing",
				},
				{
					desc:        "invalid email",
					contentType: "application/json",
					body:        []byte(`{"email":"abc", "password":"123"}`),
					wantCode:    http.StatusBadRequest,
					wantErr:     "accounts: invalid email",
				},
				{
					desc:        "email is missing",
					contentType: "application/x-www-form-urlencoded",
					body:        []byte(url.Values{"password": {"123"}}.Encode()),
					wantCode:    http.StatusBadRequest,
					wantErr:     "accounts: email is missing",
				},
				{
					desc:        "missing password",
					contentType: "application/json",
					body:        []byte(`{"email":"a@b", "password":""}`),
					wantCode:    http.StatusBadRequest,
					wantErr:     "accounts: password is missing",
				},
				{
					desc:        "valid",
					contentType: "application/x-www-form-urlencoded",
					body:        []byte(url.Values{"email": {"a@b"}, "password": {"123"}}.Encode()),
					wantCode:    http.StatusNoContent,
					wantErr:     "",
				},
				{
					desc:        "wrong password",
					contentType: "application/json",
					body:        []byte(`{"email":"a@b", "password":"456"}`),
					wantCode:    http.StatusBadRequest,
					wantErr:     "accounts: invalid email or password",
				},
				{
					desc:        "unknown email",
					contentType: "application/json",
					body:        []byte(`{"email":"b@c", "password":"456"}`),
					wantCode:    http.StatusBadRequest,
					wantErr:     "accounts: invalid email or password",
				},
			}
			for _, c := range cases {
				t.Run(c.desc, func(t *testing.T) {
					accts.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
						code := errors.Code(err)
						if code == 0 {
							code = http.StatusInternalServerError
						}
						if c.wantErr != "" {
							assert.ErrorContains(t, err, c.wantErr)
						} else {
							assert.NoError(t, err)
						}
						w.WriteHeader(code)
					}

					res, err := http.Post(srv.URL+"/login", c.contentType, bytes.NewReader(c.body))
					require.NoError(t, err)
					require.Equal(t, c.wantCode, res.StatusCode)
				})
			}
		})
	}
}

func BenchmarkFailedLogin(b *testing.B) {
	cases := []struct {
		name  string
		setup func() pgdb.Pool
	}{
		{"pgx", func() pgdb.Pool { db := testdb.NewPgx(b, "", ""); return pgxadapt.ToPool(db) }},
		{"sql", func() pgdb.Pool { db := testdb.NewSQL(b, "", ""); return sqladapt.ToPool(db) }},
		{"pq", func() pgdb.Pool { db := testdb.NewPqSQL(b, "", ""); return sqladapt.ToPool(db) }},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			pool := tc.setup()
			accts, srv := setupAccounts(b, pool)
			accts.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
				code := errors.Code(err)
				if code == 0 {
					code = http.StatusInternalServerError
				}
				w.WriteHeader(code)
			}

			// create a valid account for "a@b"
			createAccount(b, srv.URL, "a@b", "123")

			b.Run("known", func(b *testing.B) {
				body := []byte(`{"email":"a@b", "password":"456"}`)
				for b.Loop() {
					res, err := http.Post(srv.URL+"/login", "application/json", bytes.NewReader(body))
					if err != nil {
						b.FailNow()
					}
					if res.StatusCode != 400 {
						b.FailNow()
					}
				}
			})

			b.Run("unknown", func(b *testing.B) {
				body := []byte(`{"email":"b@c", "password":"456"}`)
				for b.Loop() {
					res, err := http.Post(srv.URL+"/login", "application/json", bytes.NewReader(body))
					if err != nil {
						b.FailNow()
					}
					if res.StatusCode != 400 {
						b.FailNow()
					}
				}
			})
		})
	}
}
