package acctmw

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/tokens"
	"github.com/google/uuid"
)

// Session is a middleware that ensures a session is always present for the
// wrapped handler. It loads the logged-in account or anonymous session based
// on the session cookie, if present, or generates a new anonymous session
// (that will be lazily-created if session data is needed) if none is present.
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

		var (
			ssnID   string
			ssnData json.RawMessage
			acct    *accounts.Account
		)
		switch {
		case tok == nil:
			// no active session, will create an anonymous one

		case tok.RefID == uuid.Nil:
			// this is an existing anonymous session
			ssnID = tok.Token
			ssnData = tok.Data

		default:
			// this is an authenticated session, treat as no session if account
			// not found
			var err error
			acct, err = accounts.ByID(ctx, a.Conn, tok.RefID)
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
				// no active session, will create an anonymous one
			} else {
				// authenticated account was found, continue with this active session
				ssnID = tok.Token
				ssnData = tok.Data
			}
		}

		if acct != nil {
			ctx = acctctx.WithAccount(ctx, acct)
		}
		ctx = acctctx.WithSession(ctx, ssnID, ssnData)
		r = r.WithContext(ctx)

		// call the wrapped handler
		h.ServeHTTP(w, r)

		// if the session data was modified, it needs to be saved back to the DB
		ctx = r.Context()
		if ssnID, ssnData, isDirty := acctctx.Session(ctx); isDirty {
			if ssnID == "" {
				// generate new anonymous session
				if _, stop := a.generateNewAnonymousSession(w, r, ssnData); stop {
					return
				}
			} else {
				// update data of an existing session
				if err := a.Tokens.UpdateData(ctx, ssnID, ssnData); err != nil {
					a.ErrorHandler(w, r, err)
					return
				}
			}
		}
	})
}

func (a *Accounts) generateNewAnonymousSession(w http.ResponseWriter, r *http.Request, data json.RawMessage) (newr *http.Request, stop bool) {
	// the anonymous session token is valid for a short duration and has idle
	// expiration but its cookie is always session-scoped (deleted when browser
	// is closed)
	ssnTok, ssnData, err := a.Tokens.New(r.Context(), tokens.TokenArgs{
		Type:           a.sessionTokenType(),
		RefID:          uuid.Nil,
		AbsoluteExpiry: anonymousSessionDuration,
		IdleExpiry:     idleAnonymousSessionDuration,
		Data:           data,
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
