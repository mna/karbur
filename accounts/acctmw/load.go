package acctmw

import (
	"database/sql"
	"net/http"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/tokens"
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
				acct, err := accounts.ByID(r.Context(), a.Conn, tok.RefID)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					a.ErrorHandler(w, r, err)
					return
				}

				// if account does not exist, do as if no session cookie was present
				if acct != nil {
					ctx := acctctx.WithAccount(r.Context(), acct)
					ctx = acctctx.WithSessionID(ctx, tok.Token)
					r = r.WithContext(ctx)
				}
			}
		}
		h.ServeHTTP(w, r)
	})
}
