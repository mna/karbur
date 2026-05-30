// Package accounts provides a substantial implementation of the
// security-sensitive and often subtle flows related to accounts: registration
// and deletion, login and logout, reset password, verify and change email, and
// a simple group-based authorization mechanism.
package accounts

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/migrate"
	"codeberg.org/mna/karbur/server/params"
	"github.com/alexedwards/argon2id"
)

//go:embed migrations
var migrations embed.FS

// RegisterMigrations registers the accounts package's migrations with the
// provided migrator. The migrations are registered under the group
// "karbur/accounts" and depends on "karbur/tokens".
func RegisterMigrations(mig *migrate.Migrator) error {
	root, _ := fs.Sub(migrations, "migrations")
	return mig.Register("karbur/accounts", nil, root, "karbur/tokens")
}

type Accounts struct {
	// Conn is a database connection (which is satisfied by pgdb.Pool, it doesn't
	// need to be a dedicated connection, it just needs to satisfy the
	// interface).
	Conn pgdb.Connection

	// ParamsDecoder is the params.Decoder to use to decode HTTP parameters.
	ParamsDecoder *params.Decoder

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
}

const AccountsTag = errors.ErrorTag("accounts")

type Action string

const (
	ActionRegister Action = "register"
	ActionLogin    Action = "login"
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
