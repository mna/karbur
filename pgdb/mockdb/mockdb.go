package mockdb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mna/karbur/pgdb"
)

// ErrNotMocked is the error returned by default when an operation has
// not been mocked.
var ErrNotMocked = errors.New("operation not mocked")

// NewPool creates a mock Pool with the provided arguments, which may be
// function signatures that match specific operations of the interface (e.g.
// func(context.Context) (pgdb.Connection, error) to use as the Conn
// method), or types that implement all or a subset of the interfaces, in which
// case the implemented methods are used as functions for the mock.
//
// It calls NewConnection to build the connection part of the Pool interface,
// so args handling of NewConnection also applies.
//
// As a special case for NewPool, if an arg is of type pgdb.Connection and
// no valid value has been assigned to ConnFunc yet, a pool.ConnFunc function
// is generated to return that pgdb.Connection value.
func NewPool(args ...interface{}) *Pool {
	conn := NewConnection(args...)
	pool := &Pool{
		Connection: conn,
	}
	for _, arg := range args {
		switch v := arg.(type) {
		case pgdb.Connection:
			if pool.ConnFunc == nil {
				pool.ConnFunc = func(context.Context) (pgdb.Connection, error) {
					return v, nil
				}
			}
		case func(context.Context) (pgdb.Connection, error):
			pool.ConnFunc = v
		case interface {
			Conn(context.Context) (pgdb.Connection, error)
		}:
			pool.ConnFunc = v.Conn
		}
	}
	return pool
}

// Pool implements a mockable pgdb.Pool. By defaut, it returns
// ErrNotMocked for all operations.
type Pool struct {
	pgdb.Connection
	ConnFunc func(context.Context) (pgdb.Connection, error)
}

func (p *Pool) Conn(ctx context.Context) (pgdb.Connection, error) {
	if p.ConnFunc != nil {
		return p.ConnFunc(ctx)
	}
	return nil, ErrNotMocked
}

// NewConnection creates a mock Connection with the provided arguments, which
// may be function signatures that match specific operations of the interface
// (e.g.  func() error to use as the Close method), or types that implement all
// or a subset of the interfaces, in which case the implemented methods are
// used as functions for the mock.
//
// It calls NewQueryer and NewBeginTxer to build the Queryer and BeginTxer
// parts of the Connection interface, so args handling of those functions also
// applies.
func NewConnection(args ...interface{}) *Connection {
	q := NewQueryer(args...)
	btx := NewBeginTxer(args...)
	conn := &Connection{
		Queryer:   q,
		BeginTxer: btx,
	}
	for _, arg := range args {
		switch v := arg.(type) {
		case func() error:
			conn.CloseFunc = v
		case interface {
			Close() error
		}:
			conn.CloseFunc = v.Close
		}
	}
	return conn
}

// Connection implements a mockable pgdb.Connection. By defaut, it returns
// ErrNotMocked for all operations.
type Connection struct {
	pgdb.Queryer
	pgdb.BeginTxer
	CloseFunc func() error
}

func (c *Connection) Close() error {
	if c.CloseFunc != nil {
		return c.CloseFunc()
	}
	return ErrNotMocked
}

// NewTxer creates a mock Txer with the provided arguments, which may be
// function signatures that match specific operations of the interface (e.g.
// func(context.Context) error to use as the Commit or Rollback methods), or
// types that implement all or a subset of the interfaces, in which case the
// implemented methods are used as functions for the mock. When function
// signatures are used, the first matching signature is applied to CommitFunc,
// and the next (and any subsequent ones) to RollbackFunc.
//
// It calls NewQueryer to build the queryer part of the Txer interface, so args
// handling of NewQueryer also applies.
func NewTxer(args ...interface{}) *Txer {
	q := NewQueryer(args...)
	txer := &Txer{
		Queryer: q,
	}
	for _, arg := range args {
		switch v := arg.(type) {
		case func(context.Context) error:
			if txer.CommitFunc == nil {
				txer.CommitFunc = v
			} else {
				txer.RollbackFunc = v
			}
		default:
			if v, ok := v.(interface {
				Commit(context.Context) error
			}); ok {
				txer.CommitFunc = v.Commit
			}
			if v, ok := v.(interface {
				Rollback(context.Context) error
			}); ok {
				txer.RollbackFunc = v.Rollback
			}
		}
	}
	return txer
}

// Txer implements a mockable pgdb.Txer. By defaut, it returns
// ErrNotMocked for all operations.
type Txer struct {
	pgdb.Queryer
	CommitFunc   func(context.Context) error
	RollbackFunc func(context.Context) error
}

func (t *Txer) Commit(ctx context.Context) error {
	if t.CommitFunc != nil {
		return t.CommitFunc(ctx)
	}
	return ErrNotMocked
}

func (t *Txer) Rollback(ctx context.Context) error {
	if t.RollbackFunc != nil {
		return t.RollbackFunc(ctx)
	}
	return ErrNotMocked
}

// NewCursor creates a mock Cursor with the provided arguments, which may be
// function signatures that match specific operations of the interface (e.g.
// func() error to use as the Close or Err methods), or
// types that implement all or a subset of the interfaces, in which case the
// implemented methods are used as functions for the mock. When function
// signatures are used, the first matching signature is applied to CloseFunc,
// and the next (and any subsequent ones) to ErrFunc.
func NewCursor(args ...interface{}) *Cursor {
	cur := &Cursor{}
	for _, arg := range args {
		switch v := arg.(type) {
		case func() error:
			if cur.CloseFunc == nil {
				cur.CloseFunc = v
			} else {
				cur.ErrFunc = v
			}
		case func() bool:
			cur.NextFunc = v
		case func(interface{}) error:
			cur.ScanFunc = v
		default:
			if v, ok := v.(interface{ Close() error }); ok {
				cur.CloseFunc = v.Close
			}
			if v, ok := v.(interface{ Err() error }); ok {
				cur.ErrFunc = v.Err
			}
			if v, ok := v.(interface{ Next() bool }); ok {
				cur.NextFunc = v.Next
			}
			if v, ok := v.(interface{ Scan(interface{}) error }); ok {
				cur.ScanFunc = v.Scan
			}
		}
	}
	return cur
}

// Cursor implements a mockable pgdb.Cursor. By defaut, it returns
// ErrNotMocked for all operations.
type Cursor struct {
	CloseFunc func() error
	ErrFunc   func() error
	NextFunc  func() bool
	ScanFunc  func(interface{}) error
}

func (c *Cursor) Close() error {
	if c.CloseFunc != nil {
		return c.CloseFunc()
	}
	return ErrNotMocked
}

func (c *Cursor) Err() error {
	if c.ErrFunc != nil {
		return c.ErrFunc()
	}
	return ErrNotMocked
}

func (c *Cursor) Next() bool {
	if c.NextFunc != nil {
		return c.NextFunc()
	}
	return false
}

func (c *Cursor) Scan(dst interface{}) error {
	if c.ScanFunc != nil {
		return c.ScanFunc(dst)
	}
	return ErrNotMocked
}

// NewBeginTxer creates a mock BeginTxer with the provided arguments, which may
// be function signatures that match specific operations of the interface (e.g.
// func(context.Context, *sql.TxOptions) (pgdb.Txer, error) to use as the
// BeginTxFunc function), or types that implement all or a subset of the
// interfaces, in which case the implemented methods are used as functions for
// the mock.
//
// As a special case for NewBeginTxer, if an arg is of type pgdb.Txer and
// no valid value has been assigned to BeginTxFunc yet, a BeginTxer.BeginTxFunc
// function is generated to return that value.
func NewBeginTxer(args ...interface{}) *BeginTxer {
	btx := &BeginTxer{}
	for _, arg := range args {
		switch v := arg.(type) {
		case pgdb.Txer:
			if btx.BeginTxFunc == nil {
				btx.BeginTxFunc = func(context.Context, *sql.TxOptions) (pgdb.Txer, error) {
					return v, nil
				}
			}
		case func(context.Context, *sql.TxOptions) (pgdb.Txer, error):
			btx.BeginTxFunc = v
		case interface {
			BeginTx(context.Context, *sql.TxOptions) (pgdb.Txer, error)
		}:
			btx.BeginTxFunc = v.BeginTx
		}
	}
	return btx
}

// BeginTxer implements a mockable pgdb.BeginTxer. By default, it
// returns ErrNotMocked for all operations.
type BeginTxer struct {
	BeginTxFunc func(context.Context, *sql.TxOptions) (pgdb.Txer, error)
}

func (b *BeginTxer) BeginTx(ctx context.Context, opts *sql.TxOptions) (pgdb.Txer, error) {
	if b.BeginTxFunc != nil {
		return b.BeginTxFunc(ctx, opts)
	}
	return nil, ErrNotMocked
}

// NewQueryer creates a mock Queryer with the provided arguments, which may be
// function signatures that match specific operations of the interface (e.g.
// func(context.Context, string, ...interface{}) (sql.Result, error) to use as
// the Exec method), or types that implement all or a subset of the interfaces,
// in which case the implemented methods are used as functions for the mock.
// When function signatures are used, the first matching signature is applied
// to QueryOneFunc, and the next (and any subsequent ones) to QueryManyFunc.
func NewQueryer(args ...interface{}) *Queryer {
	q := &Queryer{}
	for _, arg := range args {
		switch v := arg.(type) {
		case func(context.Context, string, ...interface{}) (sql.Result, error):
			q.ExecFunc = v
		case func(context.Context, interface{}, string, ...interface{}) error:
			if q.QueryOneFunc == nil {
				q.QueryOneFunc = v
			} else {
				q.QueryManyFunc = v
			}
		case func(context.Context, string, ...interface{}) pgdb.Cursor:
			q.CursorFunc = v
		case func(interface{}) bool:
			q.AsFunc = v
		case pgdb.Queryer:
			q.ExecFunc = v.Exec
			q.QueryOneFunc = v.QueryOne
			q.QueryManyFunc = v.QueryMany
			q.CursorFunc = v.Cursor
			q.AsFunc = v.As
		default:
			if v, ok := v.(interface {
				Exec(context.Context, string, ...interface{}) (sql.Result, error)
			}); ok {
				q.ExecFunc = v.Exec
			}
			if v, ok := v.(interface {
				QueryOne(context.Context, interface{}, string, ...interface{}) error
			}); ok {
				q.QueryOneFunc = v.QueryOne
			}
			if v, ok := v.(interface {
				QueryMany(context.Context, interface{}, string, ...interface{}) error
			}); ok {
				q.QueryManyFunc = v.QueryMany
			}
			if v, ok := v.(interface {
				Cursor(context.Context, string, ...interface{}) pgdb.Cursor
			}); ok {
				q.CursorFunc = v.Cursor
			}
			if v, ok := v.(interface {
				As(interface{}) bool
			}); ok {
				q.AsFunc = v.As
			}
		}
	}
	return q
}

// Queryer implements a mockable pgdb.Queryer. By default, it
// returns ErrNotMocked for all operations.
type Queryer struct {
	ExecFunc      func(context.Context, string, ...interface{}) (sql.Result, error)
	QueryOneFunc  func(context.Context, interface{}, string, ...interface{}) error
	QueryManyFunc func(context.Context, interface{}, string, ...interface{}) error
	CursorFunc    func(context.Context, string, ...interface{}) pgdb.Cursor
	AsFunc        func(interface{}) bool
}

func (q *Queryer) Exec(ctx context.Context, stmt string, args ...interface{}) (sql.Result, error) {
	if q.ExecFunc != nil {
		return q.ExecFunc(ctx, stmt, args...)
	}
	return nil, ErrNotMocked
}

func (q *Queryer) QueryOne(ctx context.Context, dst interface{}, stmt string, args ...interface{}) error {
	if q.QueryOneFunc != nil {
		return q.QueryOneFunc(ctx, dst, stmt, args...)
	}
	return ErrNotMocked
}

func (q *Queryer) QueryMany(ctx context.Context, dst interface{}, stmt string, args ...interface{}) error {
	if q.QueryManyFunc != nil {
		return q.QueryManyFunc(ctx, dst, stmt, args...)
	}
	return ErrNotMocked
}

func (q *Queryer) Cursor(ctx context.Context, stmt string, args ...interface{}) pgdb.Cursor {
	if q.CursorFunc != nil {
		return q.CursorFunc(ctx, stmt, args...)
	}
	return &Cursor{}
}

func (q *Queryer) As(i interface{}) bool {
	if q.AsFunc != nil {
		return q.AsFunc(i)
	}
	return false
}
