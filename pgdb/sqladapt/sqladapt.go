// Package sqladapt provides functions to adapt types for the stdlib's
// database/sql package to the common abstract database interfaces.
//
// The types supported in calls to As must be:
// - **sql.DB for a pgdb.Pool
// - **sql.Conn for a pgdb.Connection
// - **sql.Tx for a pgdb.Txer
package sqladapt

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/georgysavva/scany/v2/sqlscan"
	"codeberg.org/mna/karbur/pgdb"
)

// ToPool converts the stdlib's *sql.DB type to the common pgdb.Pool
// interface type.
func ToPool(sqldb *sql.DB) pgdb.Pool {
	return &db{db: sqldb}
}

type db struct {
	db *sql.DB
}

func (d *db) As(i interface{}) bool {
	p, ok := i.(**sql.DB)
	if !ok {
		return false
	}
	*p = d.db
	return true
}

func (d *db) BeginTx(ctx context.Context, opts *sql.TxOptions) (pgdb.Txer, error) {
	sqltx, err := d.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &tx{tx: sqltx}, nil
}

func (d *db) Close() error {
	return d.db.Close()
}

func (d *db) Conn(ctx context.Context) (pgdb.Connection, error) {
	sqlconn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return &conn{conn: sqlconn}, nil
}

func (d *db) Exec(ctx context.Context, stmt string, args ...interface{}) (sql.Result, error) {
	return d.db.ExecContext(ctx, stmt, args...)
}

func (d *db) QueryOne(ctx context.Context, dst interface{}, stmt string, args ...interface{}) error {
	err := sqlscan.Get(ctx, d.db, dst, stmt, args...)
	if sqlscan.NotFound(err) {
		return fmt.Errorf("not found: %w", sql.ErrNoRows)
	}
	return err
}

func (d *db) QueryMany(ctx context.Context, dst interface{}, stmt string, args ...interface{}) error {
	return sqlscan.Select(ctx, d.db, dst, stmt, args...)
}

func (d *db) Cursor(ctx context.Context, stmt string, args ...interface{}) pgdb.Cursor {
	rows, err := d.db.QueryContext(ctx, stmt, args...)
	return &cursor{rows: rows, initErr: err}
}

type conn struct {
	conn *sql.Conn
}

func (c *conn) As(i interface{}) bool {
	p, ok := i.(**sql.Conn)
	if !ok {
		return false
	}
	*p = c.conn
	return true
}

func (c *conn) BeginTx(ctx context.Context, opts *sql.TxOptions) (pgdb.Txer, error) {
	sqltx, err := c.conn.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &tx{tx: sqltx}, nil
}

func (c *conn) Close() error {
	return c.conn.Close()
}

func (c *conn) Exec(ctx context.Context, stmt string, args ...interface{}) (sql.Result, error) {
	return c.conn.ExecContext(ctx, stmt, args...)
}

func (c *conn) QueryOne(ctx context.Context, dst interface{}, stmt string, args ...interface{}) error {
	err := sqlscan.Get(ctx, c.conn, dst, stmt, args...)
	if sqlscan.NotFound(err) {
		return fmt.Errorf("not found: %w", sql.ErrNoRows)
	}
	return err
}

func (c *conn) QueryMany(ctx context.Context, dst interface{}, stmt string, args ...interface{}) error {
	return sqlscan.Select(ctx, c.conn, dst, stmt, args...)
}

func (c *conn) Cursor(ctx context.Context, stmt string, args ...interface{}) pgdb.Cursor {
	rows, err := c.conn.QueryContext(ctx, stmt, args...)
	return &cursor{rows: rows, initErr: err}
}

type tx struct {
	tx *sql.Tx
}

func (t *tx) As(i interface{}) bool {
	p, ok := i.(**sql.Tx)
	if !ok {
		return false
	}
	*p = t.tx
	return true
}

func (t *tx) Exec(ctx context.Context, stmt string, args ...interface{}) (sql.Result, error) {
	return t.tx.ExecContext(ctx, stmt, args...)
}

func (t *tx) QueryOne(ctx context.Context, dst interface{}, stmt string, args ...interface{}) error {
	err := sqlscan.Get(ctx, t.tx, dst, stmt, args...)
	if sqlscan.NotFound(err) {
		return fmt.Errorf("not found: %w", sql.ErrNoRows)
	}
	return err
}

func (t *tx) QueryMany(ctx context.Context, dst interface{}, stmt string, args ...interface{}) error {
	return sqlscan.Select(ctx, t.tx, dst, stmt, args...)
}

func (t *tx) Cursor(ctx context.Context, stmt string, args ...interface{}) pgdb.Cursor {
	rows, err := t.tx.QueryContext(ctx, stmt, args...)
	return &cursor{rows: rows, initErr: err}
}

func (t *tx) Commit(_ context.Context) error {
	return t.tx.Commit()
}

func (t *tx) Rollback(_ context.Context) error {
	return t.tx.Rollback()
}

type cursor struct {
	rows    *sql.Rows
	initErr error
}

func (c *cursor) Close() error {
	if c.rows != nil {
		return c.rows.Close()
	}
	return nil
}

func (c *cursor) Err() error {
	if c.rows != nil {
		return c.rows.Err()
	}
	return c.initErr
}

func (c *cursor) Next() bool {
	if c.rows != nil {
		return c.rows.Next()
	}
	return false
}

func (c *cursor) Scan(dst interface{}) error {
	if c.rows != nil {
		return sqlscan.ScanRow(dst, c.rows)
	}
	return c.initErr
}
