// Package acctmw provides a substantial implementation of the
// security-sensitive and often subtle flows related to accounts: registration
// and deletion, login and logout, reset password, verify and change email, and
// a simple group-based authorization mechanism.
package acctmw

import (
	"net/http"
	"strings"
	"time"

	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/server/params"
	"codeberg.org/mna/karbur/tokens"
	"github.com/alexedwards/argon2id"
)

type Accounts struct {
	// Conn is a database connection (which is satisfied by pgdb.Pool, it doesn't
	// need to be a dedicated connection, it just needs to satisfy the
	// interface).
	Conn pgdb.Connection

	// Tokens is the tokens manager to use for secure, random tokens.
	Tokens *tokens.Tokens

	// ParamsDecoder is the params.Decoder to use to decode HTTP parameters.
	ParamsDecoder *params.Decoder

	// AllowRememberMe indicates if the "remember_me" field is supported in the
	// login flow. If so, and if "remember_me" is true on login, the session is
	// persistent and valid for 30 days, otherwise the session expires in 12
	// hours and the cookie does not persist.
	AllowRememberMe bool

	// ErrorHandler is called to render the response in case of an error in one
	// of the Accounts middleware handlers.
	//
	// The error can be queried for meta-information to help proper handling:
	//   * its code (via errors.Code) is the recommended HTTP status code to use
	//   * its "parameter" key (via errors.KeyValue) indicates the specific
	//   parameter that failed the validation, if any
	//   * its "action" key (via errors.KeyValue) indicates the action that
	//   failed, e.g. "register" (see the Action constants)
	//
	// Internal server errors (e.g. database unreachable) are not tagged and do
	// not typically carry additional information, so the fallback should be to
	// render a 500 response.
	ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

	// Argon2Params are the argon2 parameters to use to hash passwords for
	// storage and verification. If nil, argon2id.DefaultParams are used.
	Argon2Params *argon2id.Params

	// SessionTokenType is the token type used for the session token. Defaults to
	// "session" if empty.
	SessionTokenType string
}

func (a *Accounts) sessionTokenType() string {
	if a.SessionTokenType != "" {
		return a.SessionTokenType
	}
	return defaultSessionTokenType
}

const (
	AccountsTag = errors.ErrorTag("accounts")

	defaultSessionTokenType = "session"
	shortSessionDuration    = 12 * time.Hour
	longSessionDuration     = 30 * 24 * time.Hour
)

type Action string

const (
	ActionRegister Action = "register"
	ActionLogin    Action = "login"
	ActionLoad     Action = "load"
)

func validateEmail(email string, act Action) error {
	before, after, _ := strings.Cut(email, "@")
	if before == "" || after == "" {
		if email == "" {
			return errors.TagNew("email is missing", AccountsTag,
				"code", "400", "parameter", "email", "action", string(act))
		}
		return errors.TagNew("invalid email", AccountsTag,
			"code", "400", "parameter", "email", "action", string(act))
	}
	return nil
}

func validatePassword(pwd string, act Action) error {
	if pwd == "" {
		return errors.TagNew("password is missing", AccountsTag,
			"code", "400", "parameter", "password", "action", string(act))
	}
	return nil
}
