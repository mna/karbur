package accounts

import (
	"context"
	"net/http"
	"strings"

	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"github.com/alexedwards/argon2id"
	"github.com/jackc/pgerrcode"
)

type registerInput struct {
	Email     string `schema:"email" json:"email"`
	Password  string `schema:"password" json:"password"`
	Password2 string `schema:"password2" json:"password2"`
}

func (i *registerInput) Validate() error {
	before, after, _ := strings.Cut(i.Email, "@")
	if before == "" || after == "" {
		if i.Email == "" {
			return errors.TagNew("email is missing", AccountsTag,
				"code", "400", "parameter", "email", "action", string(ActionRegister))
		}
		return errors.TagNew("invalid email", AccountsTag,
			"code", "400", "parameter", "email", "action", string(ActionRegister))
	}
	if i.Password == "" {
		return errors.TagNew("password is missing", AccountsTag,
			"code", "400", "parameter", "password", "action", string(ActionRegister))
	}
	if i.Password != i.Password2 {
		return errors.TagNew("passwords do not match", AccountsTag,
			"code", "400", "parameter", "password2", "action", string(ActionRegister))
	}
	return nil
}

func (a *Accounts) Register() func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var input registerInput
			if err := a.ParamsDecoder.Decode(r, &input); err != nil {
				if !errors.IsTag(err, AccountsTag) {
					err = errors.Tag(err, AccountsTag, "code", "400", "action", string(ActionRegister))
				}
				a.ErrorHandler(w, r, err)
				return
			}

			if _, err := a.register(r.Context(), input.Email, input.Password); err != nil {
				a.ErrorHandler(w, r, err)
				return
			}
			h.ServeHTTP(w, r)
		})
	}
}

func (a *Accounts) register(ctx context.Context, email, password string) (int64, error) {
	const insertAccount = `
INSERT INTO
  "accounts_accounts" (
    "email",
    "password"
  )
VALUES
  ($1, $2)
RETURNING
  "id"
`
	var id int64
	params := a.Argon2Params
	if params == nil {
		params = argon2id.DefaultParams
	}
	hashedPwd, err := argon2id.CreateHash(password, params)
	if err != nil {
		return id, err
	}

	err = pgdb.EnsureQueryer(ctx, a.Conn, func(ctx context.Context, q pgdb.Queryer) error {
		return q.QueryOne(ctx, &id, insertAccount, email, hashedPwd)
	})
	if err != nil && pgdb.SQLState(err) == pgerrcode.UniqueViolation {
		return id, errors.Tag(err, AccountsTag,
			"code", "409", "parameter", "email", "actions", string(ActionRegister))
	}
	return id, err
}
