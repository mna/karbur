// Package accounts provides a substantial implementation of the
// security-sensitive and often subtle flows related to accounts: registration
// and deletion, login and logout, reset password, verify and change email, and
// a simple group-based authorization mechanism.
package accounts

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/migrate"
	"github.com/jackc/pgerrcode"
)

// AccountsTag is the error tag used to tag validation errors in this package.
// Untagged errors are typically internal server errors in HTTP vocabulary.
const AccountsTag = errors.ErrorTag("accounts")

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

// TODO: SetGroups, AddGroup, RemoveGroup, return groups when returning
// account, test those DB-based functions directly. Eventually, SetPassword,
// VerifyEmail, SetEmail.

// ByEmail returns the account corresponding to the email. If none exist, the
// error is sql.ErrNoRows (check with errors.Is).
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

// ByID returns the account corresponding to the primary key identifier. If
// none exist, the error is sql.ErrNoRows (check with errors.Is).
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

// Create creates a new account with the provided email and hashed password. It
// returns a properly-tagged error if database validations fail (such as
// account already existing or email or password length error).
func Create(ctx context.Context, q pgdb.Queryer, email, hashedPwd string) (*Account, error) {
	const insertAccount = `
INSERT INTO
  "accounts_accounts" (
    "email",
    "password"
  )
VALUES
  ($1, $2)
RETURNING
  "id"
`
	var acct *Account
	err := pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		var id int64
		err := q.QueryOne(ctx, &id, insertAccount, email, hashedPwd)
		if err != nil {
			return err
		}

		acct, err = ByID(ctx, q, id)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		switch pgdb.SQLState(err) {
		case pgerrcode.UniqueViolation:
			return nil, errors.TagNew("an account already exists for this email", AccountsTag,
				"code", fmt.Sprint(http.StatusConflict), "parameter", "email")

		case pgerrcode.CheckViolation:
			if perr := pgdb.AsProtocolError(err); perr != nil {
				switch perr.ConstraintName {
				case "chk_email_length":
					return nil, errors.TagNew("email is too long", AccountsTag,
						"code", fmt.Sprint(http.StatusBadRequest), "parameter", "email")
				case "chk_password_length":
					// this is treated as a server error as it is caused by the hashing algorithm
					return nil, errors.TagNew("password hash is too long", AccountsTag,
						"code", fmt.Sprint(http.StatusInternalServerError), "parameter", "password")
				}
			}
		}
	}

	return acct, err
}

// Delete deletes the account identified by the primary key id.
func Delete(ctx context.Context, q pgdb.Queryer, id int64) error {
	const deleteAccount = `
DELETE
FROM
	"accounts_accounts"
WHERE
	"id" = $1
`
	return pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		_, err := q.Exec(ctx, deleteAccount, id)
		return err
	})
}

// CreateGroups ensures the specified groups are created if they do not already
// exist.
func CreateGroups(ctx context.Context, q pgdb.Queryer, groups []string) error {
	const insertGroups = `
INSERT INTO
	"accounts_groups" (
		"name"
	)
SELECT * FROM UNNEST($1::text[])
ON CONFLICT ON CONSTRAINT uidx_groups_name DO NOTHING
`
	return pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		_, err := q.Exec(ctx, insertGroups, groups)
		return err
	})
}

// Groups returns the list of existing group names.
func Groups(ctx context.Context, q pgdb.Queryer) ([]string, error) {
	const selectGroups = `
SELECT
	"name"
FROM
	"accounts_groups"
ORDER BY
	"name"
`
	var groups []string
	err := pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		return q.QueryMany(ctx, &groups, selectGroups)
	})
	return groups, err
}

// SetGroups sets the account's group membership to exactly the provided
// groups. Note that non-existing groups are silently ignored.
func SetGroups(ctx context.Context, btx pgdb.BeginTxer, acctID int64, groups []string) error {
	const (
		removeMembers = `
DELETE
FROM
	"accounts_members" m
USING
	"accounts_groups" g
WHERE
	m.group_id = g.id AND
	g.account_id = $1 AND
	g.name != ALL($2)
`

		insertMembers = `
INSERT INTO
	"accounts_members" (
		account_id,
		group_id
	)
SELECT
	$1,
	g.id
FROM
	"accounts_groups" g
WHERE
	g.name = ANY($2)
ON CONFLICT ON CONSTRAINT uidx_members_account_id_group_id DO NOTHING
`
	)

	return pgdb.EnsureTx(ctx, btx, func(ctx context.Context, tx pgdb.Txer) error {
		if _, err := tx.Exec(ctx, removeMembers, acctID, groups); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, insertMembers, acctID, groups); err != nil {
			return err
		}
		return nil
	})
}

// AddGroup adds the specified group to the membership of the account, if
// necessary. Note that non-existing groups are silently ignored.
func AddGroup(ctx context.Context, q pgdb.Queryer, acctID int64, group string) error {
	const insertMember = `
INSERT INTO
	"accounts_members" (
		account_id,
		group_id
	)
SELECT
	$1,
	g.id
FROM
	"accounts_groups" g
WHERE
	g.name = $2
ON CONFLICT ON CONSTRAINT uidx_members_account_id_group_id DO NOTHING
`
	return pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		_, err := q.Exec(ctx, insertMember, acctID, group)
		return err
	})
}

// RemoveGroup removes the specified group from the membership of the account,
// if necessary. Note that non-existing groups are silently ignored.
func RemoveGroup(ctx context.Context, q pgdb.Queryer, acctID int64, group string) error {
	const removeMember = `
DELETE
FROM
	"accounts_members" m
USING
	"accounts_groups" g
WHERE
	m.group_id = g.id AND
	g.account_id = $1 AND
	g.name = $2
`
	return pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		_, err := q.Exec(ctx, removeMember, acctID, group)
		return err
	})
}
