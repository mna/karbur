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

// TODO: the login, logout and delete middleware must also ensure that after
// changing the authenticated session, they generate an anonymous session
// immediately for the wrapped handler to use (and this Session middleware must
// correctly set any session data updates to the new anonymous session).

// Session is a middleware that ensures a session is always present for the
// wrapped handler. It loads the logged-in account based on the session cookie,
// if present, or the anonymous session, and generates a new anonymous session
// if none is present. The only exception is if a server error occurs, the
// ErrorHandler may be called without an active session.
func (a *Accounts) Session(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tok *tokens.Token
		ctx := r.Context()

		// load the existing session token if present
		if ck, _ := r.Cookie("__Host-ssn"); ck != nil {
			ssnTok := ck.Value

			var err error
			tok, err = a.Tokens.Verify(ctx, ssnTok, tokens.MustMatchType(a.sessionTokenType()))
			if err != nil && !errors.Is(err, tokens.ErrInvalid) {
				a.ErrorHandler(w, r, err)
				return
			}
			// an invalid/expired token will have tok == nil, which is as if not present
		}

		switch {
		case tok == nil:
			// no active session, create an anonymous one
			newr, stop := a.generateAnonymousSession(w, r)
			if stop {
				return
			}
			r = newr

		case tok.RefID == uuid.Nil:
			// this is an existing anonymous session
			ctx = acctctx.WithSession(ctx, tok.Token, tok.Data)
			r = r.WithContext(ctx)

		default:
			// this is an authenticated session, treat as no session if account
			// not found
			acct, err := accounts.ByID(ctx, a.Conn, tok.RefID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				a.ErrorHandler(w, r, err)
				return
			}

			if acct == nil {
				// for security, delete all session tokens for this ref id, as the
				// account does not exist.
				if err := a.Tokens.DeleteByTypeRef(ctx, a.sessionTokenType(), tok.RefID); err != nil {
					a.ErrorHandler(w, r, err)
					return
				}

				// no active session, create an anonymous one
				newr, stop := a.generateAnonymousSession(w, r)
				if stop {
					return
				}
				r = newr

			} else {
				// authenticated account was found, continue with this active session
				ctx = acctctx.WithAccount(ctx, acct)
				ctx = acctctx.WithSession(ctx, tok.Token, tok.Data)
				r = r.WithContext(ctx)
			}
		}

		// call the wrapped handler
		h.ServeHTTP(w, r)

		// if the session data was modified, it needs to be saved back to the DB
		ctx = r.Context()
		if ssnID, ssnData, isDirty := acctctx.Session(ctx); isDirty {
			if err := a.Tokens.UpdateData(ctx, ssnID, ssnData); err != nil {
				a.ErrorHandler(w, r, err)
				return
			}
		}
	})
}

func (a *Accounts) generateAnonymousSession(w http.ResponseWriter, r *http.Request) (newr *http.Request, stop bool) {
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
		return r, true
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
	return r, false
}
