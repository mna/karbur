package acctmw

import (
	"context"
	"fmt"
	"net/http"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
	"github.com/alexedwards/argon2id"
)

type deleteInput struct {
	Password string `schema:"password" json:"password"`
}

func (i *deleteInput) Validate() error {
	if err := validatePassword(i.Password, ActionLogin); err != nil {
		return err
	}
	return nil
}

func (a *Accounts) Delete(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input deleteInput
		if err := a.ParamsDecoder.Decode(r, &input); err != nil {
			if !errors.IsTag(err, accounts.AccountsTag) {
				err = errors.Tag(err, accounts.AccountsTag, "code", "400", "action", string(ActionDelete))
			}
			a.ErrorHandler(w, r, err)
			return
		}

		// this requires a currently logged-in account
		acct := acctctx.Account(r.Context())
		if acct == nil {
			err := errors.TagNew("permission denied", accounts.AccountsTag,
				"code", fmt.Sprint(http.StatusForbidden), "action", string(ActionDelete))
			a.ErrorHandler(w, r, err)
			return
		}
		if err := a.delete(r.Context(), input.Password, acct.Password); err != nil {
			a.ErrorHandler(w, r, err)
			return
		}
		// TODO: delete account, delete session tokens and any pending reset/verify tokens, clear session cookie
	})
}

func (a *Accounts) delete(ctx context.Context, password, passwordHash string) error {
	ok, err := argon2id.ComparePasswordAndHash(password, passwordHash)
	if err != nil {
		return err
	}
	if !ok {
		return errors.TagNew("invalid password", accounts.AccountsTag,
			"code", "400", "parameter", "password", "action", string(ActionDelete))
	}
	return nil
}
