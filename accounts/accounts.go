// Package accounts provides a substantial implementation of the
// security-sensitive and often subtle flows related to accounts: registration
// and deletion, login and logout, reset password, verify and change email, and
// a simple group-based authorization mechanism.
package accounts

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"time"

	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/migrate"
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

// Account represents a database account. If its email is verified, the
// Verified field is not null (Verified.Valid is true), otherwise it is yet to
// be verified.
type Account struct {
	ID       int64               `db:"id"`
	Email    string              `db:"email"`
	Password string              `db:"password"`
	Verified sql.Null[time.Time] `db:"verified"`
	Created  time.Time           `db:"created"`
}

func ByEmail(ctx context.Context, q pgdb.Queryer, email string) (*Account, error) {
	const selectAccount = `
SELECT
	"id",
	"email",
	"password",
	"verified",
	"created"
FROM
	"accounts_accounts"
WHERE
	"email" = $1
`
	var acct Account
	err := pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		return q.QueryOne(ctx, &acct, selectAccount, email)
	})
	if err != nil {
		return nil, err
	}
	return &acct, nil
}

func ByID(ctx context.Context, q pgdb.Queryer, id int64) (*Account, error) {
	const selectAccount = `
SELECT
	"id",
	"email",
	"password",
	"verified",
	"created"
FROM
	"accounts_accounts"
WHERE
	"id" = $1
`
	var acct Account
	err := pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		return q.QueryOne(ctx, &acct, selectAccount, id)
	})
	if err != nil {
		return nil, err
	}
	return &acct, nil
}
