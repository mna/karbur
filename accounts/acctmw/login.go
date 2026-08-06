package acctmw

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
			if !errors.IsTag(err, accounts.AccountsTag) {
				err = errors.Tag(err, accounts.AccountsTag, "code", fmt.Sprint(http.StatusBadRequest), "action", string(ActionLogin))
			}
			a.ErrorHandler(w, r, err)
			return
		}
		if input.RememberMe && !a.AllowRememberMe {
			err := errors.TagNew("invalid parameter", accounts.AccountsTag,
				"code", fmt.Sprint(http.StatusBadRequest), "parameter", "remember_me", "action", string(ActionLogin))
			a.ErrorHandler(w, r, err)
			return
		}

		acct, err := a.login(r.Context(), input.Email, input.Password)
		if err != nil {
			a.ErrorHandler(w, r, err)
			return
		}

		// create the session token and the cookie to store it, the logic regarding
		// the existing session vs the new logged-in one is as follows:
		//   * if there was already an authenticated session, replace it with a brand
		//   new one and delete the old authenticated session
		//   * if there was a saved anonymous session (session id is not empty),
		//   rotate it to the authenticated one, keeping its data
		//   * if there was an unsaved anonymous session (session id is empty),
		//   create a new one with the existing session data
		expiry, maxAge := authenticatedSessionDurations(input.RememberMe)

		ctx := r.Context()

		var (
			createNewSession bool
			sessionData      json.RawMessage
		)
		oldSessionID := acctctx.SessionID(ctx)
		if oldAcct := acctctx.Account(ctx); oldAcct != nil {
			// login while an(other?) account was already logged in, delete the
			// previous session and create a new one
			if oldSessionID != "" {
				if err := a.Tokens.Delete(ctx, oldSessionID); err != nil {
					a.ErrorHandler(w, r, err)
					return
				}
			}
			createNewSession = true
			sessionData = nil
		} else {
			// anonymous session, create new session if it is an unsaved anonymous
			// session
			createNewSession = oldSessionID == ""

			// keep existing data from memory
			_, data, _ := acctctx.Session(ctx)
			sessionData = data
		}

		var newSessionID string
		if createNewSession {
			var err error
			newSessionID, sessionData, err = a.Tokens.New(ctx, tokens.TokenArgs{
				Type:           a.sessionTokenType(),
				RefID:          acct.ID,
				AbsoluteExpiry: expiry,
				Data:           sessionData,
			})
			if err != nil {
				a.ErrorHandler(w, r, err)
				return
			}
		} else {
			var err error
			newSessionID, err = a.Tokens.Rotate(ctx, oldSessionID, tokens.TokenArgs{
				RefID:          acct.ID,
				AbsoluteExpiry: expiry,
				Data:           sessionData,
			})
			if err != nil {
				a.ErrorHandler(w, r, err)
				return
			}
		}

		// store the logged-in account and session in the context for subsequent
		// middleware
		acctctx.ResetSession(ctx, newSessionID, sessionData)
		ctx = acctctx.WithAccount(ctx, acct)
		r = r.WithContext(ctx)

		http.SetCookie(w, &http.Cookie{
			Name:     "__Host-ssn",
			Value:    newSessionID,
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
			return nil, errors.TagNew("invalid email or password", accounts.AccountsTag,
				"code", "400", "parameter", "password", "action", string(ActionLogin))
		}
		return nil, err
	}

	ok, err := argon2id.ComparePasswordAndHash(password, acct.Password)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.TagNew("invalid email or password", accounts.AccountsTag,
			"code", "400", "parameter", "password", "action", string(ActionLogin))
	}
	return acct, nil
}

func authenticatedSessionDurations(rememberMe bool) (expiry time.Duration, maxAge int) {
	// without remember me, the cookie is session-scoped (no max age)
	expiry = shortSessionDuration
	if rememberMe {
		expiry = longSessionDuration
		maxAge = int(expiry / time.Second)
	}
	return expiry, maxAge
}
