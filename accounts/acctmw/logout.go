package acctmw

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
)

// Logout is a middleware that logs out the currently logged-in account.
func (a *Accounts) Logout(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// decode parameters in an empty struct to ensure there are no invalid
		// arguments provided.
		var input struct{}
		if err := a.ParamsDecoder.Decode(r, &input); err != nil {
			if !errors.IsTag(err, accounts.AccountsTag) {
				err = errors.Tag(err, accounts.AccountsTag, "code", "400", "action", string(ActionLogout))
			}
			a.ErrorHandler(w, r, err)
			return
		}

		ssnID := acctctx.SessionID(r.Context())
		acct := acctctx.Account(r.Context())
		if ssnID == "" || acct == nil {
			err := errors.TagNew("permission denied", accounts.AccountsTag, "code", fmt.Sprint(http.StatusForbidden),
				"action", string(ActionLogout))
			a.ErrorHandler(w, r, err)
			return
		}

		if err := a.logout(r.Context(), ssnID); err != nil {
			a.ErrorHandler(w, r, err)
			return
		}

		// clear the logged-in account and reset the session from the context for
		// subsequent handlers
		acctctx.ResetSession(r.Context(), "", nil)
		ctx := acctctx.WithAccount(r.Context(), nil)
		r = r.WithContext(ctx)

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

		h.ServeHTTP(w, r)
	})
}

func (a *Accounts) logout(ctx context.Context, ssnID string) error {
	// at the database level, the only action is to delete the token via the
	// Tokens manager.
	return a.Tokens.Delete(ctx, ssnID)
}
