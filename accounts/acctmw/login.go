package acctmw

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/tokens"
	"github.com/alexedwards/argon2id"
)

type loginInput struct {
	Email      string `schema:"email" json:"email"`
	Password   string `schema:"password" json:"password"`
	RememberMe bool   `schema:"remember_me" json:"remember_me"`
}

func (i *loginInput) Validate() error {
	if err := validateEmail(i.Email, ActionLogin); err != nil {
		return err
	}
	if err := validatePassword(i.Password, ActionLogin); err != nil {
		return err
	}
	return nil
}

func (a *Accounts) Login(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input loginInput
		if err := a.ParamsDecoder.Decode(r, &input); err != nil {
			if !errors.IsTag(err, AccountsTag) {
				err = errors.Tag(err, AccountsTag, "code", "400", "action", string(ActionLogin))
			}
			a.ErrorHandler(w, r, err)
			return
		}
		if input.RememberMe && !a.AllowRememberMe {
			err := errors.TagNew("invalid parameter", AccountsTag,
				"code", "400", "parameter", "remember_me", "action", string(ActionLogin))
			a.ErrorHandler(w, r, err)
			return
		}

		acct, err := a.login(r.Context(), input.Email, input.Password)
		if err != nil {
			a.ErrorHandler(w, r, err)
			return
		}

		// create the session token and the cookie to store it
		var maxAge int
		dur := shortSessionDuration
		if input.RememberMe {
			dur = longSessionDuration
			maxAge = int(dur / time.Second)
		}
		ssnTok, err := a.Tokens.New(r.Context(), tokens.TokenArgs{Type: a.sessionTokenType(), RefID: acct.ID, Expiry: dur})
		if err != nil {
			a.ErrorHandler(w, r, err)
			return
		}

		// store the logged-in account and session ID in the context for subsequent
		// middleware
		ctx := acctctx.WithAccount(r.Context(), acct)
		ctx = acctctx.WithSessionID(ctx, ssnTok)
		r = r.WithContext(ctx)

		http.SetCookie(w, &http.Cookie{
			Name:     "__Host-ssn",
			Value:    ssnTok,
			Path:     "/",
			MaxAge:   maxAge,
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		h.ServeHTTP(w, r)
	})
}

const failPwdHash = "$argon2id$v=19$m=65536,t=1,p=8$u/bcVmH/87u/sZTTdq1Wdg$BWJfiHsq6IvDEF8PSPE+UnNxV7vdafKSQtIXVmdG4Ro"

func (a *Accounts) login(ctx context.Context, email, password string) (*accounts.Account, error) {
	acct, err := accounts.ByEmail(ctx, a.Conn, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// do a password-hash check that is ignored, to help prevent timing
			// attacks when the account does not exist (see BenchmarkFailedLogin)
			_, _ = argon2id.ComparePasswordAndHash(password, failPwdHash)
			return nil, errors.TagNew("invalid email or password", AccountsTag,
				"code", "400", "parameter", "password", "action", string(ActionLogin))
		}
		return nil, err
	}

	ok, err := argon2id.ComparePasswordAndHash(password, acct.Password)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.TagNew("invalid email or password", AccountsTag,
			"code", "400", "parameter", "password", "action", string(ActionLogin))
	}
	return acct, nil
}
