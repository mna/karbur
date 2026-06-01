package acctmw

import (
	"context"
	"net/http"

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
	if err := validateEmail(i.Email, ActionRegister); err != nil {
		return err
	}
	if err := validatePassword(i.Password, ActionRegister); err != nil {
		return err
	}
	if i.Password != i.Password2 {
		return errors.TagNew("passwords do not match", AccountsTag,
			"code", "400", "parameter", "password2", "action", string(ActionRegister))
	}
	return nil
}

func (a *Accounts) Register(h http.Handler) http.Handler {
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
	if err != nil {
		switch pgdb.SQLState(err) {
		case pgerrcode.UniqueViolation:
			return id, errors.TagNew("an account already exists for this email", AccountsTag,
				"code", "409", "parameter", "email", "actions", string(ActionRegister))

		case pgerrcode.CheckViolation:
			if perr := pgdb.AsProtocolError(err); perr != nil {
				switch perr.ConstraintName {
				case "chk_email_length":
					return id, errors.TagNew("email is too long", AccountsTag,
						"code", "400", "parameter", "email", "actions", string(ActionRegister))
				case "chk_password_length":
					return id, errors.TagNew("password hash is too long", AccountsTag,
						"code", "500", "parameter", "password", "actions", string(ActionRegister))
				}
			}
		}
	}
	return id, err
}
