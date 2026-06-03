package acctmw

import (
	"context"
	"net/http"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/errors"
	"github.com/alexedwards/argon2id"
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
		return errors.TagNew("passwords do not match", accounts.AccountsTag,
			"code", "400", "parameter", "password2", "action", string(ActionRegister))
	}
	return nil
}

func (a *Accounts) Register(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input registerInput
		if err := a.ParamsDecoder.Decode(r, &input); err != nil {
			if !errors.IsTag(err, accounts.AccountsTag) {
				err = errors.Tag(err, accounts.AccountsTag, "code", "400", "action", string(ActionRegister))
			}
			a.ErrorHandler(w, r, err)
			return
		}

		if err := a.register(r.Context(), input.Email, input.Password); err != nil {
			a.ErrorHandler(w, r, err)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (a *Accounts) register(ctx context.Context, email, password string) error {
	hashedPwd, err := argon2id.CreateHash(password, a.argon2Params())
	if err != nil {
		return err
	}

	_, err = accounts.Create(ctx, a.Conn, email, hashedPwd)
	if err != nil {
		if errors.IsTag(err, accounts.AccountsTag) {
			err = errors.WithKeyValue(err, "action", string(ActionRegister))
		}
	}
	return err
}
