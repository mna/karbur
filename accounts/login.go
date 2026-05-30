package accounts

import (
	"context"
	"net/http"

	"codeberg.org/mna/karbur/errors"
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
		h.ServeHTTP(w, r)
	})
}

func (a *Accounts) login(ctx context.Context, email, password string) error {
	panic("unimplemented")
}
