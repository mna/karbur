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
	// of the Accounts middleware handlers. The error can be queried for its Code
	// via errors.Code for the recommended HTTP status code to use
	ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)
}

type registerInput struct {
	Email     string `schema:"email" json:"email"`
	Password  string `schema:"password" json:"password"`
	Password2 string `schema:"password2" json:"password2"`
}

func (i *registerInput) Validate() error {
	before, after, _ := strings.Cut(i.Email, "@")
	if before == "" || after == "" {
		if i.Email == "" {
			return errors.New("email is missing")
		}
		return errors.New("invalid email")
	}
	if i.Password == "" {
		return errors.New("password is missing")
	}
	if i.Password != i.Password2 {
		return errors.New("passwords do not match")
	}
	return nil
}

func (a *Accounts) Register() func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var input registerInput
			if err := a.ParamsDecoder.Decode(r, &input); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// TODO: support creating with a generated password, and require changing
			// it on first login? Support login with an email token?
		})
	}
}
