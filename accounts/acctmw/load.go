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

// Load is a middleware that loads the logged-in account based on the session
// cookie, if present, so that subsequent handlers have access to the account.
func (a *Accounts) Load(h http.Handler) http.Handler {
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
		if ssnData, isDirty := acctctx.SessionData(r.Context()); isDirty {
			// TODO: save back the session data to the DB
			_ = ssnData
		}
	})
}
