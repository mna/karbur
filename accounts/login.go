package accounts

import (
	"context"
	"database/sql"
	"net/http"

	"codeberg.org/mna/karbur/errors"
	"github.com/alexedwards/argon2id"
)

type loginInput struct {
	Email    string `schema:"email" json:"email"`
	Password string `schema:"password" json:"password"`
}

func (i *loginInput) Validate() error {
	if err := validateEmail(i.Email, ActionLogin); err != nil {
		return err
	}
	if err := validatePassword(i.Password, ActionLogin); err != nil {
		return err
	}
	return nil
}

func (a *Accounts) Login(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input loginInput
		if err := a.ParamsDecoder.Decode(r, &input); err != nil {
			if !errors.IsTag(err, AccountsTag) {
				err = errors.Tag(err, AccountsTag, "code", "400", "action", string(ActionLogin))
			}
			a.ErrorHandler(w, r, err)
			return
		}

		if err := a.login(r.Context(), input.Email, input.Password); err != nil {
			a.ErrorHandler(w, r, err)
			return
		}
		// TODO: insert logged-in user in context, create session
		h.ServeHTTP(w, r)
	})
}

const failPwdHash = "$argon2id$v=19$m=65536,t=1,p=8$u/bcVmH/87u/sZTTdq1Wdg$BWJfiHsq6IvDEF8PSPE+UnNxV7vdafKSQtIXVmdG4Ro"

func (a *Accounts) login(ctx context.Context, email, password string) error {
	acct, err := a.ByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// do a password-hash check that is ignored, to help prevent timing
			// attacks when the account does not exist (see BenchmarkFailedLogin)
			_, _ = argon2id.ComparePasswordAndHash(password, failPwdHash)
			return errors.TagNew("invalid email or password", AccountsTag,
				"code", "400", "parameter", "password", "action", string(ActionLogin))
		}
		return err
	}

	ok, err := argon2id.ComparePasswordAndHash(password, acct.Password)
	if err != nil {
		return err
	}
	if !ok {
		return errors.TagNew("invalid email or password", AccountsTag,
			"code", "400", "parameter", "password", "action", string(ActionLogin))
	}
	return nil
}
