package acctmw

import (
	"database/sql"
	"net/http"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/tokens"
	"github.com/google/uuid"
)

// TODO: rename this middleware to Session, make it ensure that the wrapped
// handler is always called with a valid session (anonymous or not). Then the
// login, logout and delete middleware must also ensure that after changing the
// authenticated session, they generate an anonymous session immediately for
// the wrapped handler to use.

// Session is a middleware that ensures a session is always present for the
// wrapped handler. It loads the logged-in account based on the session cookie,
// if present, or the anonymous session, and generates a new anonymous session
// if none is present. The only exception is if a server error occurs, the
// ErrorHandler may be called without an active session.
func (a *Accounts) Session(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ck, _ := r.Cookie("__Host-ssn"); ck != nil {
			ssnTok := ck.Value
			// invalid tokens are treated as if not present
			tok, err := a.Tokens.Verify(r.Context(), ssnTok, tokens.MustMatchType(a.sessionTokenType()))
			if err != nil && !errors.Is(err, tokens.ErrInvalid) {
				a.ErrorHandler(w, r, err)
				return
			}

			if tok != nil {
				ctx := r.Context()

				if tok.RefID == uuid.Nil {
					// this is an anonymous session
					ctx = acctctx.WithSessionID(ctx, tok.Token)
					ctx = acctctx.WithSessionData(ctx, tok.Data)
					r = r.WithContext(ctx)
				} else {
					// this is not an anonymous session, treat as no session if account not found
					acct, err := accounts.ByID(ctx, a.Conn, tok.RefID)
					if err != nil && !errors.Is(err, sql.ErrNoRows) {
						a.ErrorHandler(w, r, err)
						return
					}
					if acct != nil {
						ctx = acctctx.WithAccount(ctx, acct)
						ctx = acctctx.WithSessionID(ctx, tok.Token)
						ctx = acctctx.WithSessionData(ctx, tok.Data)
						r = r.WithContext(ctx)
					}
				}
			}
		}

		// call the wrapped handler
		h.ServeHTTP(w, r)

		// if the session data was modified, it needs to be saved back to the DB
		ctx := r.Context()
		if ssnData, ssnID, isDirty := acctctx.SessionData(ctx); isDirty {
			// TODO: after some operations (e.g. going from anonymous to logged in),
			// the session ID might've changed between before and after the wrapped
			// handler was called, and we have no way of getting that new session id.
			ssnID := acctctx.SessionID(ctx)
			if err := a.Tokens.UpdateData(ctx, ssnID, ssnData); err != nil {
				a.ErrorHandler(w, r, err)
				return
			}
		}
	})
}

func (a *Accounts) generateAnonymousSession(w http.ResponseWriter, r *http.Request) *http.Request {
	// the anonymous session token is valid for a short duration and has idle
	// expiration but its cookie is always session-scoped (deleted when browser
	// is closed)
	ssnTok, ssnData, err := a.Tokens.New(r.Context(), tokens.TokenArgs{
		Type:           a.sessionTokenType(),
		RefID:          uuid.Nil,
		AbsoluteExpiry: anonymousSessionDuration,
		IdleExpiry:     idleAnonymousSessionDuration,
	})
	if err != nil {
		a.ErrorHandler(w, r, err)
		return r
	}

	// store the session in the context for subsequent middleware
	ctx := acctctx.WithSession(r.Context(), ssnTok, ssnData)
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
	return r
}
