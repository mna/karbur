// Package tokens implements a random, secure and time-limited token generator
// that helps implement common features like session IDs (multi-use,
// long-lived), password resets and verify email (both single-use, short-lived)
// scenarios.
package tokens

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/base64"
	"time"

	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/migrate"
)

//go:embed migrations
var migrations embed.FS

// RegisterMigrations registers the tokens package's migrations with the
// provided migrator.
func RegisterMigrations(mig *migrate.Migrator) error {
	return mig.Register("karbur/tokens", nil, migrations)
}

// TokenArgs configures the token to create.
type TokenArgs struct {
	// Type is an application-defined type of token.
	Type string
	// RefID is an application-defined identifier linked to this token.
	RefID int64
	// SingleUse indicates if the token is unique and single-use (consumed and
	// invalid after first use, a single valid one exists for the Type and RefID)
	// or not.
	SingleUse bool
	// Expiry is the duration that the token is valid. It is precise to the
	// second.
	Expiry time.Duration
}

const rawTokenSize = 32

// New generates a new random, secure token configured according to args.
// It uses the existing DB transaction if there is one, otherwise it uses q.
// The token is base64-url-encoded so it is safe to use in URLs and cookies if
// needed.
//
// For single-use tokens, if a token already exists for the same Type and
// RefID, it is replaced by the new token, invalidating the previous one.
func New(ctx context.Context, q pgdb.Queryer, args TokenArgs) (string, error) {
	b := make([]byte, rawTokenSize)
	_, _ = rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)

	const insertToken = `
INSERT INTO
  "tokens_tokens" (
    "token",
    "type",
    "single_use",
    "ref_id",
    "expiry"
  )
VALUES
  ($1, $2, $3, $4, now() + $5 * interval '1 second')
ON CONFLICT ("type", "ref_id") WHERE "single_use" DO
UPDATE SET
  "token" = EXCLUDED."token",
  "expiry" = EXCLUDED."expiry"
`
	err := pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		_, err := q.Exec(ctx, insertToken, token, args.Type, args.SingleUse, args.RefID, int64(args.Expiry/time.Second))
		return err
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// Token represents a token loaded via Verify.
type Token struct {
	Token     string    `db:"token"`
	Type      string    `db:"type"`
	SingleUse bool      `db:"single_use"`
	RefID     int64     `db:"ref_id"`
	Expiry    time.Time `db:"expiry"`
}

// ErrInvalid is the error returned if an invalid (expired or unknown) token is
// passed to Verify.
const ErrInvalid = errors.ConstError("invalid token")

// Verify loads and verifies if the provided token is valid. If the token is
// single-use, it is deleted after load as it is not valid anymore. It uses the
// existing DB transaction if there is one, otherwise starts one with btx.
//
// The vfn argument is extra validation to apply on the token before
// considering it valid. The MustMatchType and MustMatchTypeAndRefID functions
// in this package generate common validation functions for use in those
// scenarios:
//   - If the token is multi-use, such as a session ID, MustMatchType should be
//     used to verify it has the expected type (so that a token generated for a
//     different scenario cannot be valid for a session check)
//   - If the token is single-use, such as a password reset,
//     MustMatchTypeAndRefID should be used to verify it has the expected type
//     and ref_id (which should be known, as the user typically enters the
//     account's email address in addition to the reset token).
func Verify(ctx context.Context, btx pgdb.BeginTxer, token string, vfn func(*Token) error) (*Token, error) {
	const getToken = `
SELECT
  "token",
  "type",
  "single_use",
  "ref_id",
  "expiry"
FROM
  "tokens_tokens"
WHERE
  "token" = $1 AND
  "expiry" > now()
`
	var tok Token
	err := pgdb.EnsureTx(ctx, btx, func(ctx context.Context, tx pgdb.Txer) error {
		if err := tx.QueryOne(ctx, &tok, getToken, token); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalid
			}
			return err
		}

		// if the token is single-use, delete it as it is now consumed
		if tok.SingleUse {
			if err := Delete(ctx, tx, token); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if vfn != nil {
		if err := vfn(&tok); err != nil {
			return nil, err
		}
	}
	return &tok, nil
}

func MustMatchType(t string) func(*Token) error {
	return func(tok *Token) error {
		if tok.Type != t {
			return ErrInvalid
		}
		return nil
	}
}

func MustMatchTypeAndRefID(t string, refID int64) func(*Token) error {
	return func(tok *Token) error {
		if tok.Type != t || tok.RefID != refID {
			return ErrInvalid
		}
		return nil
	}
}

// Delete deletes the specified token, regardless of its expiry. It uses
// the existing DB transaction if there is one, otherwise it uses q.
func Delete(ctx context.Context, q pgdb.Queryer, token string) error {
	const deleteToken = `
DELETE FROM
  "tokens_tokens"
WHERE
  "token" = $1
`
	return pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		_, err := q.Exec(ctx, deleteToken, token)
		return err
	})
}

// DeleteByTypeRef deletes all tokens with the specified tokenType and
// tokenRefID. This is meant for scenarios like when all session IDs should be
// invalidated for a given user, or canceling a password reset operation. It
// uses the existing DB transaction if there is one, otherwise it uses q.
func DeleteByTypeRef(ctx context.Context, q pgdb.Queryer, tokenType string, tokenRefID int64) error {
	const deleteTokens = `
DELETE FROM
  "tokens_tokens"
WHERE
  "ref_id" = $1 AND
  "type" = $2
`
	return pgdb.EnsureQueryer(ctx, q, func(ctx context.Context, q pgdb.Queryer) error {
		_, err := q.Exec(ctx, deleteTokens, tokenRefID, tokenType)
		return err
	})
}
