// Package pgdb streamlines the database API by exposing only the context-aware
// version of methods and enhances querying by supporting scanning into structs
// and slices. It abstract the various postgresql database drivers that may be
// used, the standard library's database/sql and github.com/jackc/pgx/v5 are
// supported and the sqladapt or pgxadapt packages can be used to convert from
// the specific type to the abstraction.
//
// At any moment, it is possible to break out of the abstraction and use the
// specific implementation by calling Queryer.As, which is implemented by all
// the main types (Pool, Connection, and Txer).
package pgdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Pool defines the methods required for a database pool.
type Pool interface {
	Connection
	Conn(context.Context) (Connection, error)
}

// BeginTxer defines the method required for a type that can begin
// transactions.
type BeginTxer interface {
	BeginTx(context.Context, *sql.TxOptions) (Txer, error)
}

// Connection defines the methods required to act as a database
// connection.
type Connection interface {
	Queryer
	BeginTxer
	Close() error
}

// Txer is a database transaction. It implements Queryer, and adds methods
// to Commit or Rollback the transaction. Calling Rollback on a committed
// transaction returns an error and is otherwise a no-op, so a useful idiom
// is to defer a call to Rollback after starting a transaction, and call
// Commit when needed, which will invalidate the Rollback.
type Txer interface {
	Queryer
	Commit(context.Context) error
	Rollback(context.Context) error
}

// Cursor implement an efficient, iterable database result. Typical usage
// is to use an inifinite for loop and exit when Next returns false.
// It must be closed after use. Consult Err to find any error that may
// have caused early exit from the loop.
type Cursor interface {
	Close() error
	Err() error
	Next() bool
	Scan(interface{}) error
}

// Queryer is the common interface to query and execute SQL
// statements.
type Queryer interface {
	As(interface{}) bool
	Exec(context.Context, string, ...interface{}) (sql.Result, error)
	QueryOne(context.Context, interface{}, string, ...interface{}) error
	QueryMany(context.Context, interface{}, string, ...interface{}) error
	Cursor(context.Context, string, ...interface{}) Cursor
}

type ctxKey int

const (
	ctxTx ctxKey = iota
)

func setCtxTx(ctx context.Context, tx Txer) context.Context {
	return context.WithValue(ctx, ctxTx, tx)
}

func getCtxTx(ctx context.Context) (Txer, bool) {
	tx, ok := ctx.Value(ctxTx).(Txer)
	return tx, ok
}

// Tx begins a transaction with btx and calls fn with the Txer. If fn returns
// an error, the transaction is rolled back and that error is returned from Tx,
// otherwise the transaction is committed and nil is returned. The context
// passed to fn should always be used inside fn.
func Tx(ctx context.Context, btx BeginTxer, opts *sql.TxOptions, fn func(context.Context, Txer) error) error {
	tx, err := btx.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ctx = setCtxTx(ctx, tx)
	if err := fn(ctx, tx); err != nil {
		rberr := tx.Rollback(ctx)
		return errors.Join(err, rberr)
	}
	return tx.Commit(ctx)
}

// EnsureTx makes sure that fn is called inside a transaction. It either uses
// an existing transaction from the context, or starts a new one with btx.
//
// If fn returns an error, the transaction is rolled back and that error is
// returned from EnsureTx, otherwise the transaction is committed only if a new
// one was started by EnsureTx and nil is returned. The context passed to fn
// should always be used inside fn.
func EnsureTx(ctx context.Context, btx BeginTxer, fn func(context.Context, Txer) error) error {
	tx, _ := getCtxTx(ctx)
	if tx == nil {
		return Tx(ctx, btx, nil, fn)
	}
	if err := fn(ctx, tx); err != nil {
		rberr := tx.Rollback(ctx)
		return errors.Join(err, rberr)
	}
	return nil
}

// EnsureQueryer makes sure that fn is called with a Queryer. It either uses
// the existing transaction from the context, or passes on the queryer q.
//
// If fn returns an error, that error is returned from EnsureQueryer, otherwise
// nil is returned. The context passed to fn should always be used inside fn.
// No transaction management is done by EnsureQueryer (no Commit nor Rollback).
func EnsureQueryer(ctx context.Context, q Queryer, fn func(context.Context, Queryer) error) error {
	tx, _ := getCtxTx(ctx)
	if tx != nil {
		q = tx
	}
	return fn(ctx, q)
}

// ErrNoTx is the error returned by RequireTx if no current transaction is
// available.
var ErrNoTx = errors.New("no current transaction")

// RequireTx calls fn with the existing transaction from the context, or
// returns an error if there is no active transaction.
//
// If fn returns an error, the transaction is rolled back and that error is
// returned from RequireTx, otherwise nil is returned. The context passed to fn
// should always be used inside fn. It never commits the transaction, this is
// up to the caller to handle (typically by the caller that created the
// transaction).
func RequireTx(ctx context.Context, fn func(context.Context, Txer) error) error {
	tx, _ := getCtxTx(ctx)
	if tx == nil {
		return ErrNoTx
	}
	if err := fn(ctx, tx); err != nil {
		rberr := tx.Rollback(ctx)
		return errors.Join(err, rberr)
	}
	return nil
}

// QuoteEsc takes an unquoted string literal and returns it with surrounding
// single quotes and with any existing single quotes doubled so it can
// be interpolated into a SQL statement directly (e.g. concatenated or
// with plain %s formatting).
func QuoteEsc(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

var likeEscReplacer = strings.NewReplacer(
	"\\", "\\\\",
	"%", "\\%",
	"_", "\\_",
)

// LikeEsc takes a string value and returns it with any LIKE/ILIKE character
// that needs to be escaped, escaped. This means that any backslash will be
// doubled, and any underscore ('_') or percent sign ('%') will be preceded
// by a backslash.
func LikeEsc(s string) string {
	return likeEscReplacer.Replace(s)
}
