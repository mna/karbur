package acctmw

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
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
		if err := a.delete(r.Context(), acct.ID, input.Password, acct.Password); err != nil {
			a.ErrorHandler(w, r, err)
			return
		}

		// clear the session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "__Host-ssn",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		// NOTE: we deliberately do not clear the context's account, in case the
		// subsequent handlers need to do something with it (e.g. clear additional
		// account-related data).
		h.ServeHTTP(w, r)
	})
}

func (a *Accounts) delete(ctx context.Context, acctID int64, password, passwordHash string) error {
	ok, err := argon2id.ComparePasswordAndHash(password, passwordHash)
	if err != nil {
		return err
	}
	if !ok {
		return errors.TagNew("invalid password", accounts.AccountsTag,
			"code", "400", "parameter", "password", "action", string(ActionDelete))
	}

	return pgdb.EnsureTx(ctx, a.Conn, func(ctx context.Context, tx pgdb.Txer) error {
		if err := accounts.Delete(ctx, a.Conn, acctID); err != nil {
			return err
		}
		if err := a.Tokens.DeleteByTypeRef(ctx, a.sessionTokenType(), acctID); err != nil {
			return err
		}
		return nil
	})
}
