package acctmw

import (
	"net/http"

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

		if err := a.logout(r.Context()); err != nil {
			a.ErrorHandler(w, r, err)
			return
		}

		// // create the session token and the cookie to store it
		// var maxAge int
		// dur := shortSessionDuration
		// if input.RememberMe {
		// 	dur = longSessionDuration
		// 	maxAge = int(dur / time.Second)
		// }
		// ssnTok, err := a.Tokens.New(r.Context(), tokens.TokenArgs{Type: a.sessionTokenType(), RefID: acct.ID, Expiry: dur})
		// if err != nil {
		// 	a.ErrorHandler(w, r, err)
		// 	return
		// }
		//
		// // store the logged-in account and session ID in the context for subsequent
		// // middleware
		// ctx := acctctx.WithAccount(r.Context(), acct)
		// ctx = acctctx.WithSessionID(ctx, ssnTok)
		// r = r.WithContext(ctx)
		//
		// http.SetCookie(w, &http.Cookie{
		// 	Name:     "__Host-ssn",
		// 	Value:    ssnTok,
		// 	Path:     "/",
		// 	MaxAge:   maxAge,
		// 	Secure:   true,
		// 	HttpOnly: true,
		// 	SameSite: http.SameSiteLaxMode,
		// })

		h.ServeHTTP(w, r)
	})
}
