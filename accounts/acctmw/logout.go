package acctmw

import (
	"context"
	"net/http"
	"time"

	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
)

func (a *Accounts) Logout(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// decode parameters in an empty struct to ensure there are no invalid
		// arguments provided.
		var input struct{}
		if err := a.ParamsDecoder.Decode(r, &input); err != nil {
			if !errors.IsTag(err, AccountsTag) {
				err = errors.Tag(err, AccountsTag, "code", "400", "action", string(ActionLogout))
			}
			a.ErrorHandler(w, r, err)
			return
		}

		if ssnID := acctctx.SessionID(r.Context()); ssnID != "" {
			if err := a.logout(r.Context(), ssnID); err != nil {
				a.ErrorHandler(w, r, err)
				return
			}

			// clear the logged-in account and session id from the context for
			// subsequent handlers
			ctx := acctctx.WithAccount(r.Context(), nil)
			ctx = acctctx.WithSessionID(ctx, "")
			r = r.WithContext(ctx)
		}

		// clear the session cookie unconditionally, as it may still be there even
		// if there was no session id in the context (e.g. unknown session or
		// corresponding account not found)
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
