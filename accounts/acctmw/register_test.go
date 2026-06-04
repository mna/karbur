package acctmw

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/pgxadapt"
	"codeberg.org/mna/karbur/pgdb/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func TestRegister(t *testing.T) {
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
			accts, srv := setupAccounts(t, pool, nil)

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
					desc:        "missing password",
					contentType: "application/json",
					body:        []byte(`{"email":"a@b", "password":""}`),
					wantCode:    http.StatusBadRequest,
					wantErr:     "accounts: password is missing",
				},
				{
					desc:        "non-matching passwords",
					contentType: "application/json",
					body:        []byte(`{"email":"a@b", "password":"123", "password2":"345"}`),
					wantCode:    http.StatusBadRequest,
					wantErr:     "accounts: passwords do not match",
				},
				{
					desc:        "valid",
					contentType: "application/x-www-form-urlencoded",
					body:        []byte(url.Values{"email": {"a@c"}, "password": {"123"}, "password2": {"123"}}.Encode()),
					wantCode:    http.StatusNoContent,
					wantErr:     "",
				},
				{
					desc:        "invalid json",
					contentType: "application/json",
					body:        []byte(`{"email":"a@b", "password":"123}`),
					wantCode:    http.StatusBadRequest,
					wantErr:     "accounts: unexpected EOF",
				},
				{
					desc:        "email too long",
					contentType: "application/json",
					body:        fmt.Appendf(nil, `{"email":"%s@b", "password":"123", "password2":"123"}`, strings.Repeat("a", 254)),
					wantCode:    http.StatusBadRequest,
					wantErr:     `accounts: email is too long`,
				},
				{
					desc:        "duplicate",
					contentType: "application/x-www-form-urlencoded",
					body:        []byte(url.Values{"email": {"a@b"}, "password": {"123"}, "password2": {"123"}}.Encode()),
					wantCode:    http.StatusConflict,
					wantErr:     `accounts: an account already exists for this email`,
				},
				{
					desc:        "unknown field",
					contentType: "application/x-www-form-urlencoded",
					body:        []byte(url.Values{"email": {"a@d"}, "password": {"123"}, "password2": {"123"}, "what": {"true"}}.Encode()),
					wantCode:    http.StatusBadRequest,
					wantErr:     `accounts: schema: invalid path "what"`,
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

					res, err := http.Post(srv.URL+"/register", c.contentType, bytes.NewReader(c.body))
					require.NoError(t, err)
					require.Equal(t, c.wantCode, res.StatusCode)
				})
			}
		})
	}
}
