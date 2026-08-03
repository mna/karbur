package acctmw

import (
	"net/http"

	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/tokens"
	"github.com/google/uuid"
)

// Anonymous is a middleware that generates an anonymous, or "guest", session
// and stores it in a session-scoped cookie. It expects any existing session to
// be already loaded when it runs, so the Load middeware should be used in
// front of this middleware.
func (a *Accounts) Anonymous(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ssnID := acctctx.SessionID(r.Context()); ssnID == "" {
			// the anonymous session token is valid for a short duration and has idle
			// expiration but its cookie is always session-scoped (deleted when
			// browser is closed)
			ssnTok, err := a.Tokens.New(r.Context(), tokens.TokenArgs{
				Type:           a.sessionTokenType(),
				RefID:          uuid.Nil,
				AbsoluteExpiry: anonymousSessionDuration,
				IdleExpiry:     idleAnonymousSessionDuration,
			})
			if err != nil {
				a.ErrorHandler(w, r, err)
				return
			}

			// store the session ID in the context for subsequent middleware
			ctx := acctctx.WithSessionID(r.Context(), ssnTok)
			r = r.WithContext(ctx)

			http.SetCookie(w, &http.Cookie{
				Name:     "__Host-ssn",
				Value:    ssnTok,
				Path:     "/",
				MaxAge:   0,
				Secure:   true,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}

		h.ServeHTTP(w, r)
	})
}
