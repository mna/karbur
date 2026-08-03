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

				// TODO: if there is JSON data associated with the token, store it in
				// acctctx.WithSessionData. Application code can add a middleware to
				// parse it into a typed struct and store it in the ctx (and set a
				// default/empty struct value if there is currently no data). But it
				// needs to be possible to store it back and for this middleware to
				// catch the update, so maybe a acctctx.WithSessionData always stores a
				// boxed JSON value (possibly empty), and a acctctx.SetSessionData(v
				// any) updates it with the JSON-marshaled value of v. Up to the
				// application logic to call SetSessionData to update it (and it keeps
				// a flag that it was updated so the middleware knows it has to save it
				// back).

				if tok.RefID == uuid.Nil {
					// this is an anonymous session
					ctx = acctctx.WithSessionID(ctx, tok.Token)
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
						r = r.WithContext(ctx)
					}
				}
			}
		}
		h.ServeHTTP(w, r)
	})
}
