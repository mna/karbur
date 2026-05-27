// Package pgxadapt provides functions to adapt types specific for the pgx
// postgresql driver to the common abstract database interfaces.
//
// The types supported in calls to As must be:
// - **pgxpool.Pool for a pgdb.Pool
// - **pgxpool.Conn for a pgdb.Connection
// - *pgx.Tx for a pgdb.Txer
package pgxadapt

import (
	"context"
	"database/sql"
	"fmt"

	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ToPool converts the pgx-specific pool to the common pgdb.Pool
// type.
func ToPool(pool *pgxpool.Pool) pgdb.Pool {
	return &db{pool: pool}
}

// ToTxOptions converts the pgx-specific options to the common sql ones.
// Note that the DeferrableMode field is ignored as it is not supported
// by the common sql options.
func ToTxOptions(opts pgx.TxOptions) *sql.TxOptions {
	var sqlOpts sql.TxOptions
	switch opts.IsoLevel {
	case pgx.Serializable:
		sqlOpts.Isolation = sql.LevelSerializable
	case pgx.RepeatableRead:
		sqlOpts.Isolation = sql.LevelRepeatableRead
	case pgx.ReadCommitted:
		sqlOpts.Isolation = sql.LevelReadCommitted
	case pgx.ReadUncommitted:
		sqlOpts.Isolation = sql.LevelReadUncommitted
	}

	if opts.AccessMode == pgx.ReadOnly {
		sqlOpts.ReadOnly = true
	}

	if sqlOpts == (sql.TxOptions{}) {
		return nil
	}
	return &sqlOpts
}

// ToPgxTxOptions converts the common sql transaction options type to the
// pgx-specific one.
func ToPgxTxOptions(opts *sql.TxOptions) pgx.TxOptions {
	var pgxOpts pgx.TxOptions
	if opts == nil || *opts == (sql.TxOptions{}) {
		return pgxOpts
	}

	if opts.ReadOnly {
		pgxOpts.AccessMode = pgx.ReadOnly
	}
	switch opts.Isolation {
	case sql.LevelSerializable:
		pgxOpts.IsoLevel = pgx.Serializable
	case sql.LevelRepeatableRead:
		pgxOpts.IsoLevel = pgx.RepeatableRead
	case sql.LevelReadCommitted:
		pgxOpts.IsoLevel = pgx.ReadCommitted
	case sql.LevelReadUncommitted:
		pgxOpts.IsoLevel = pgx.ReadUncommitted
	}
	return pgxOpts
}

type db struct {
	pool *pgxpool.Pool
}

func (d *db) As(i any) bool {
	p, ok := i.(**pgxpool.Pool)
	if !ok {
		return false
	}
	*p = d.pool
	return true
}

func (d *db) BeginTx(ctx context.Context, opts *sql.TxOptions) (pgdb.Txer, error) {
	ptx, err := d.pool.BeginTx(ctx, ToPgxTxOptions(opts))
	if err != nil {
		return nil, err
	}
	return &tx{tx: ptx}, nil
}

func (d *db) Close() error {
	d.pool.Close()
	return nil
}

func (d *db) Conn(ctx context.Context) (pgdb.Connection, error) {
	pconn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	return &conn{conn: pconn}, nil
}

func (d *db) Exec(ctx context.Context, stmt string, args ...any) (sql.Result, error) {
	res, err := d.pool.Exec(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	return ToSQLResult(res), nil
}

func (d *db) QueryOne(ctx context.Context, dst any, stmt string, args ...any) error {
	err := pgxscan.Get(ctx, d.pool, dst, stmt, args...)
	if pgxscan.NotFound(err) {
		return fmt.Errorf("not found: %w", sql.ErrNoRows)
	}
	return err
}

func (d *db) QueryMany(ctx context.Context, dst any, stmt string, args ...any) error {
	return pgxscan.Select(ctx, d.pool, dst, stmt, args...)
}

func (d *db) Cursor(ctx context.Context, stmt string, args ...any) pgdb.Cursor {
	// fine to ignore error here, the rows will be in failed state if there was one
	rows, _ := d.pool.Query(ctx, stmt, args...)
	return &cursor{rows: rows}
}

type conn struct {
	conn *pgxpool.Conn
}

func (c *conn) As(i any) bool {
	p, ok := i.(**pgxpool.Conn)
	if !ok {
		return false
	}
	*p = c.conn
	return true
}

func (c *conn) BeginTx(ctx context.Context, opts *sql.TxOptions) (pgdb.Txer, error) {
	ptx, err := c.conn.BeginTx(ctx, ToPgxTxOptions(opts))
	if err != nil {
		return nil, err
	}
	return &tx{tx: ptx}, nil
}

func (c *conn) Close() error {
	c.conn.Release()
	return nil
}

func (c *conn) Exec(ctx context.Context, stmt string, args ...any) (sql.Result, error) {
	res, err := c.conn.Exec(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	return ToSQLResult(res), nil
}

func (c *conn) QueryOne(ctx context.Context, dst any, stmt string, args ...any) error {
	err := pgxscan.Get(ctx, c.conn, dst, stmt, args...)
	if pgxscan.NotFound(err) {
		return fmt.Errorf("not found: %w", sql.ErrNoRows)
	}
	return err
}

func (c *conn) QueryMany(ctx context.Context, dst any, stmt string, args ...any) error {
	return pgxscan.Select(ctx, c.conn, dst, stmt, args...)
}

func (c *conn) Cursor(ctx context.Context, stmt string, args ...any) pgdb.Cursor {
	// fine to ignore error here, the rows will be in failed state if there was one
	rows, _ := c.conn.Query(ctx, stmt, args...)
	return &cursor{rows: rows}
}

type tx struct {
	tx pgx.Tx
}

func (t *tx) As(i any) bool {
	p, ok := i.(*pgx.Tx)
	if !ok {
		return false
	}
	*p = t.tx
	return true
}

func (t *tx) Exec(ctx context.Context, stmt string, args ...any) (sql.Result, error) {
	res, err := t.tx.Exec(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	return ToSQLResult(res), nil
}

func (t *tx) QueryOne(ctx context.Context, dst any, stmt string, args ...any) error {
	err := pgxscan.Get(ctx, t.tx, dst, stmt, args...)
	if pgxscan.NotFound(err) {
		return fmt.Errorf("not found: %w", sql.ErrNoRows)
	}
	return err
}

func (t *tx) QueryMany(ctx context.Context, dst any, stmt string, args ...any) error {
	return pgxscan.Select(ctx, t.tx, dst, stmt, args...)
}

func (t *tx) Cursor(ctx context.Context, stmt string, args ...any) pgdb.Cursor {
	// fine to ignore error here, the rows will be in failed state if there was one
	rows, _ := t.tx.Query(ctx, stmt, args...)
	return &cursor{rows: rows}
}

func (t *tx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

func (t *tx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}

type cursor struct {
	rows pgx.Rows
}

func (c *cursor) Close() error {
	// closing may set the rows.err field, so return whatever is there.
	c.rows.Close()
	return c.rows.Err()
}

func (c *cursor) Err() error {
	return c.rows.Err()
}

func (c *cursor) Next() bool {
	return c.rows.Next()
}

func (c *cursor) Scan(dst any) error {
	return pgxscan.ScanRow(dst, c.rows)
}

// ToSQLResult converts a pgx CommandTag result as returned by Exec to
// the common sql.Result type.
func ToSQLResult(tag pgconn.CommandTag) sql.Result {
	return result(tag)
}

type result pgconn.CommandTag

func (r result) LastInsertId() (int64, error) {
	return 0, errors.New("unsupported: use postgres INSERT..RETURNING instead")
}

func (r result) RowsAffected() (int64, error) {
	return pgconn.CommandTag(r).RowsAffected(), nil
}
