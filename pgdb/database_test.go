package pgdb_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mna/karbur/pgdb"
	"github.com/mna/karbur/pgdb/pgxadapt"
	"github.com/mna/karbur/pgdb/sqladapt"
	"github.com/mna/karbur/pgdb/testdb"
)

var ctx = context.Background()

func TestPool(t *testing.T) {
	c := qt.New(t)

	cases := []struct {
		name  string
		setup func() pgdb.Pool
	}{
		{"pgx", func() pgdb.Pool { db := testdb.NewPgx(c, "", ""); return pgxadapt.ToPool(db) }},
		{"sql", func() pgdb.Pool { db := testdb.NewSQL(c, "", ""); return sqladapt.ToPool(db) }},
	}
	for _, tc := range cases {
		c.Run(tc.name, func(c *qt.C) {
			pool := tc.setup()

			c.Cleanup(func() {
				err := pool.Close()
				c.Assert(err, qt.IsNil)
			})

			c.Run("QueryOne", func(c *qt.C) {
				var name string
				err := pool.QueryOne(ctx, &name, "SELECT current_database()")
				c.Assert(err, qt.IsNil)
				c.Assert(name, qt.Contains, "test")
			})

			c.Run("ExecFail", func(c *qt.C) {
				_, err := pool.Exec(ctx, "CREATE FOO bar")
				c.Assert(err, qt.IsNotNil)
				c.Assert(err.Error(), qt.Contains, "42601")
			})

			c.Run("Pool", func(c *qt.C) {
				res, err := pool.Exec(ctx, "CREATE TABLE testint (v integer primary key)")
				c.Assert(err, qt.IsNil)
				c.Cleanup(func() {
					_, _ = pool.Exec(ctx, "DROP TABLE testint")
				})

				n, err := res.RowsAffected()
				c.Assert(err, qt.IsNil)
				c.Assert(n, qt.Equals, int64(0))
				_, err = res.LastInsertId()
				c.Assert(err, qt.IsNotNil)

				for i := 1; i <= 5; i++ {
					res, err := pool.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", i)
					c.Assert(err, qt.IsNil)
					n, err := res.RowsAffected()
					c.Assert(err, qt.IsNil)
					c.Assert(n, qt.Equals, int64(1))
				}

				c.Run("As", func(c *qt.C) {
					pgpool, sqlpool := new(pgxpool.Pool), new(sql.DB)
					pgok, sqlok := pool.As(&pgpool), pool.As(&sqlpool)
					c.Assert(pgok, qt.Not(qt.Equals), sqlok)

					if pgok {
						err := pgpool.Ping(ctx)
						c.Assert(err, qt.IsNil)
					} else {
						err := sqlpool.PingContext(ctx)
						c.Assert(err, qt.IsNil)
					}
				})

				c.Run("QueryOneStruc", func(c *qt.C) {
					var dst struct{ V int }
					err := pool.QueryOne(ctx, &dst, "SELECT v FROM testint WHERE v = $1", 2)
					c.Assert(err, qt.IsNil)
					c.Assert(dst, qt.DeepEquals, struct{ V int }{2})
				})

				c.Run("QueryOneNoRow", func(c *qt.C) {
					var dst int
					err := pool.QueryOne(ctx, &dst, "SELECT v FROM testint WHERE v = $1", -1)
					c.Assert(err, qt.IsNotNil)
					c.Assert(errors.Is(err, sql.ErrNoRows), qt.IsTrue)
				})

				c.Run("QueryMany", func(c *qt.C) {
					var ids []int
					err := pool.QueryMany(ctx, &ids, "SELECT v FROM testint")
					c.Assert(err, qt.IsNil)
					c.Assert(ids, qt.DeepEquals, []int{1, 2, 3, 4, 5})
				})

				c.Run("Cursor", func(c *qt.C) {
					cur := pool.Cursor(ctx, "SELECT v FROM testint")
					defer cur.Close()

					var ids []int
					for cur.Next() {
						var id int
						err := cur.Scan(&id)
						c.Assert(err, qt.IsNil)
						ids = append(ids, id)
					}
					err := cur.Err()
					c.Assert(err, qt.IsNil)
					c.Assert(ids, qt.DeepEquals, []int{1, 2, 3, 4, 5})
				})
			})

			c.Run("Txer", func(c *qt.C) {
				_, err := pool.Exec(ctx, "CREATE TABLE testint (v integer primary key)")
				c.Assert(err, qt.IsNil)
				c.Cleanup(func() {
					_, _ = pool.Exec(ctx, "DROP TABLE testint")
				})

				for i := 1; i <= 5; i++ {
					res, err := pool.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", i)
					c.Assert(err, qt.IsNil)
					n, err := res.RowsAffected()
					c.Assert(err, qt.IsNil)
					c.Assert(n, qt.Equals, int64(1))
				}

				c.Run("BeginCommit", func(c *qt.C) {
					tx, err := pool.BeginTx(ctx, nil)
					c.Assert(err, qt.IsNil)
					defer func() { _ = tx.Rollback(ctx) }()

					res, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 6)
					c.Assert(err, qt.IsNil)

					n, err := res.RowsAffected()
					c.Assert(err, qt.IsNil)
					c.Assert(n, qt.Equals, int64(1))
					_, err = res.LastInsertId()
					c.Assert(err, qt.IsNotNil)

					err = tx.Commit(ctx)
					c.Assert(err, qt.IsNil)

					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 6)
					c.Assert(err, qt.IsNil)
					c.Assert(ok, qt.IsTrue)
				})

				c.Run("BeginRollback", func(c *qt.C) {
					tx, err := pool.BeginTx(ctx, nil)
					c.Assert(err, qt.IsNil)
					defer func() { _ = tx.Rollback(ctx) }()

					_, err = tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 7)
					c.Assert(err, qt.IsNil)

					err = tx.Rollback(ctx)
					c.Assert(err, qt.IsNil)

					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 7)
					c.Assert(err, qt.IsNotNil)
					c.Assert(errors.Is(err, sql.ErrNoRows), qt.IsTrue)
					c.Assert(ok, qt.IsFalse)
				})

				c.Run("BeginFuncCommit", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 8)
						c.Assert(err, qt.IsNil)
						return nil
					})
					c.Assert(err, qt.IsNil)

					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 8)
					c.Assert(err, qt.IsNil)
					c.Assert(ok, qt.IsTrue)
				})

				c.Run("BeginFuncRollback", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 9)
						c.Assert(err, qt.IsNil)
						return errors.New("nope")
					})
					c.Assert(err, qt.IsNotNil)
					c.Assert(err.Error(), qt.Contains, "nope")

					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 9)
					c.Assert(err, qt.IsNotNil)
					c.Assert(errors.Is(err, sql.ErrNoRows), qt.IsTrue)
					c.Assert(ok, qt.IsFalse)
				})

				c.Run("TxerAs", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						var pgtx pgx.Tx
						sqltx := new(sql.Tx)
						pgok, sqlok := tx.As(&pgtx), tx.As(&sqltx)
						c.Assert(pgok, qt.Not(qt.Equals), sqlok)

						var i int
						if pgok {
							row := pgtx.QueryRow(ctx, "SELECT 1")
							return row.Scan(&i)
						}
						row := sqltx.QueryRow("SELECT 1")
						return row.Scan(&i)
					})
					c.Assert(err, qt.IsNil)
				})

				c.Run("TxerQueryOne", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 10)
						c.Assert(err, qt.IsNil)

						var ok bool
						err = tx.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 10)
						c.Assert(err, qt.IsNil)
						c.Assert(ok, qt.IsTrue)
						return nil
					})
					c.Assert(err, qt.IsNil)
				})

				c.Run("TxerQueryOneNoRows", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						var ok bool
						err := tx.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", -1)
						c.Assert(err, qt.IsNotNil)
						c.Assert(ok, qt.IsFalse)
						return err
					})
					c.Assert(err, qt.IsNotNil)
					c.Assert(errors.Is(err, sql.ErrNoRows), qt.IsTrue)
				})

				c.Run("TxerQueryMany", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						var ids []int
						err := tx.QueryMany(ctx, &ids, "SELECT v FROM testint WHERE v < $1", 5)
						c.Assert(err, qt.IsNil)
						c.Assert(ids, qt.DeepEquals, []int{1, 2, 3, 4})
						return nil
					})
					c.Assert(err, qt.IsNil)
				})

				c.Run("TxerCursor", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						var ids []int
						cur := tx.Cursor(ctx, "SELECT v FROM testint WHERE v < $1", 5)
						for cur.Next() {
							var id int
							err := cur.Scan(&id)
							c.Assert(err, qt.IsNil)
							ids = append(ids, id)
						}
						c.Assert(ids, qt.DeepEquals, []int{1, 2, 3, 4})
						return cur.Err()
					})
					c.Assert(err, qt.IsNil)
				})

				c.Run("TxerExecFail", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						_, err := tx.Exec(ctx, "INSERT INTO no_Such_TABLE (v) VALUES ($1)", 1)
						c.Assert(err, qt.IsNotNil)
						return err
					})
					c.Assert(err, qt.IsNotNil)
					c.Assert(err.Error(), qt.Contains, "42P01")
				})

				c.Run("EnsureTxNone", func(c *qt.C) {
					err := pgdb.EnsureTx(ctx, pool, func(ctx context.Context, tx pgdb.Txer) error {
						_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 11)
						c.Assert(err, qt.IsNil)
						return nil
					})
					c.Assert(err, qt.IsNil)

					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 11)
					c.Assert(err, qt.IsNil)
					c.Assert(ok, qt.IsTrue)
				})

				c.Run("EnsureTxExistCommit", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						err := pgdb.EnsureTx(ctx, pool, func(ctx context.Context, tx pgdb.Txer) error {
							_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 12)
							c.Assert(err, qt.IsNil)
							return nil
						})
						c.Assert(err, qt.IsNil)

						// does not exist yet outside the transaction
						var ok bool
						err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 12)
						c.Assert(errors.Is(err, sql.ErrNoRows), qt.IsTrue)
						c.Assert(ok, qt.IsFalse)

						return nil
					})
					c.Assert(err, qt.IsNil)

					// exists now, after commit
					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 12)
					c.Assert(err, qt.IsNil)
					c.Assert(ok, qt.IsTrue)
				})

				c.Run("EnsureTxExistRollback", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						err := pgdb.EnsureTx(ctx, pool, func(ctx context.Context, tx pgdb.Txer) error {
							_, err := tx.Exec(ctx, "INSERT INTO no_SUCH_Table (v) VALUES ($1)", 1)
							c.Assert(err, qt.IsNotNil)
							return err
						})
						c.Assert(err, qt.IsNotNil)
						c.Assert(err.Error(), qt.Contains, "42P01")
						return err
					})
					c.Assert(err, qt.IsNotNil)
				})

				c.Run("RequireTxNone", func(c *qt.C) {
					err := pgdb.RequireTx(ctx, func(ctx context.Context, tx pgdb.Txer) error {
						panic("should not be called")
					})
					c.Assert(err, qt.IsNotNil)
					c.Assert(errors.Is(err, pgdb.ErrNoTx), qt.IsTrue)
				})

				c.Run("RequireTxExistCommit", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						err := pgdb.RequireTx(ctx, func(ctx context.Context, tx pgdb.Txer) error {
							_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 13)
							c.Assert(err, qt.IsNil)
							return nil
						})
						c.Assert(err, qt.IsNil)

						// does not exist yet outside the transaction
						var ok bool
						err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 13)
						c.Assert(errors.Is(err, sql.ErrNoRows), qt.IsTrue)
						c.Assert(ok, qt.IsFalse)

						return nil
					})
					c.Assert(err, qt.IsNil)

					// exists now, after commit
					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 13)
					c.Assert(err, qt.IsNil)
					c.Assert(ok, qt.IsTrue)
				})

				c.Run("RequireTxExistRollback", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						err := pgdb.RequireTx(ctx, func(ctx context.Context, tx pgdb.Txer) error {
							_, err := tx.Exec(ctx, "INSERT INTO no_SUCH_Table (v) VALUES ($1)", 1)
							c.Assert(err, qt.IsNotNil)
							return err
						})
						c.Assert(err, qt.IsNotNil)
						c.Assert(err.Error(), qt.Contains, "42P01")
						return err
					})
					c.Assert(err, qt.IsNotNil)
				})

				c.Run("EnsureQueryerNone", func(c *qt.C) {
					err := pgdb.EnsureQueryer(ctx, pool, func(ctx context.Context, q pgdb.Queryer) error {
						var count int
						err := pool.QueryOne(ctx, &count, "SELECT count(*) FROM testint")
						c.Assert(err, qt.IsNil)
						c.Assert(count >= 5, qt.IsTrue)
						return nil
					})
					c.Assert(err, qt.IsNil)
				})

				c.Run("EnsureQueryerExist", func(c *qt.C) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						err := pgdb.EnsureQueryer(ctx, pool, func(ctx context.Context, q pgdb.Queryer) error {
							var count int
							err := pool.QueryOne(ctx, &count, "SELECT count(*) FROM testint")
							c.Assert(err, qt.IsNil)
							c.Assert(count >= 5, qt.IsTrue)
							return io.EOF
						})
						c.Assert(errors.Is(err, io.EOF), qt.IsTrue)
						return err
					})
					c.Assert(errors.Is(err, io.EOF), qt.IsTrue)
				})
			})

			c.Run("Conn", func(c *qt.C) {
				conn, err := pool.Conn(ctx)
				c.Assert(err, qt.IsNil)
				c.Cleanup(func() {
					err := conn.Close()
					c.Assert(err, qt.IsNil)
				})

				c.Run("As", func(c *qt.C) {
					pgconn, sqlconn := new(pgxpool.Conn), new(sql.Conn)
					pgok, sqlok := conn.As(&pgconn), conn.As(&sqlconn)
					c.Assert(pgok, qt.Not(qt.Equals), sqlok)

					if pgok {
						err := pgconn.Ping(ctx)
						c.Assert(err, qt.IsNil)
					} else {
						err := sqlconn.PingContext(ctx)
						c.Assert(err, qt.IsNil)
					}
				})

				c.Run("Exec", func(c *qt.C) {
					res, err := conn.Exec(ctx, "CREATE TABLE testint_conn (v integer primary key)")
					c.Assert(err, qt.IsNil)

					n, err := res.RowsAffected()
					c.Assert(err, qt.IsNil)
					c.Assert(n, qt.Equals, int64(0))
					_, err = res.LastInsertId()
					c.Assert(err, qt.IsNotNil)

					for i := 1; i <= 5; i++ {
						res, err := conn.Exec(ctx, "INSERT INTO testint_conn (v) VALUES ($1)", i)
						c.Assert(err, qt.IsNil)
						n, err := res.RowsAffected()
						c.Assert(err, qt.IsNil)
						c.Assert(n, qt.Equals, int64(1))
					}

					c.Run("QueryOneStruc", func(c *qt.C) {
						var dst struct{ V int }
						err := conn.QueryOne(ctx, &dst, "SELECT v FROM testint_conn WHERE v = $1", 2)
						c.Assert(err, qt.IsNil)
						c.Assert(dst, qt.DeepEquals, struct{ V int }{2})
					})

					c.Run("QueryOneNoRow", func(c *qt.C) {
						var dst int
						err := conn.QueryOne(ctx, &dst, "SELECT v FROM testint_conn WHERE v = $1", -1)
						c.Assert(err, qt.IsNotNil)
						c.Assert(errors.Is(err, sql.ErrNoRows), qt.IsTrue)
					})

					c.Run("QueryMany", func(c *qt.C) {
						var ids []int
						err := conn.QueryMany(ctx, &ids, "SELECT v FROM testint_conn")
						c.Assert(err, qt.IsNil)
						c.Assert(ids, qt.DeepEquals, []int{1, 2, 3, 4, 5})
					})

					c.Run("Cursor", func(c *qt.C) {
						cur := conn.Cursor(ctx, "SELECT v FROM testint_conn")
						defer cur.Close()

						var ids []int
						for cur.Next() {
							var id int
							err := cur.Scan(&id)
							c.Assert(err, qt.IsNil)
							ids = append(ids, id)
						}
						err := cur.Err()
						c.Assert(err, qt.IsNil)
						c.Assert(ids, qt.DeepEquals, []int{1, 2, 3, 4, 5})
					})

					c.Run("BeginCommit", func(c *qt.C) {
						tx, err := conn.BeginTx(ctx, nil)
						c.Assert(err, qt.IsNil)
						defer func() { _ = tx.Rollback(ctx) }()

						res, err := tx.Exec(ctx, "INSERT INTO testint_conn (v) VALUES ($1)", 6)
						c.Assert(err, qt.IsNil)

						n, err := res.RowsAffected()
						c.Assert(err, qt.IsNil)
						c.Assert(n, qt.Equals, int64(1))
						_, err = res.LastInsertId()
						c.Assert(err, qt.IsNotNil)

						err = tx.Commit(ctx)
						c.Assert(err, qt.IsNil)

						var ok bool
						err = conn.QueryOne(ctx, &ok, "SELECT true FROM testint_conn WHERE v = $1", 6)
						c.Assert(err, qt.IsNil)
						c.Assert(ok, qt.IsTrue)
					})

					c.Run("BeginRollback", func(c *qt.C) {
						tx, err := conn.BeginTx(ctx, nil)
						c.Assert(err, qt.IsNil)
						defer func() { _ = tx.Rollback(ctx) }()

						_, err = tx.Exec(ctx, "INSERT INTO testint_conn (v) VALUES ($1)", 7)
						c.Assert(err, qt.IsNil)

						err = tx.Rollback(ctx)
						c.Assert(err, qt.IsNil)

						var ok bool
						err = conn.QueryOne(ctx, &ok, "SELECT true FROM testint_conn WHERE v = $1", 7)
						c.Assert(err, qt.IsNotNil)
						c.Assert(errors.Is(err, sql.ErrNoRows), qt.IsTrue)
						c.Assert(ok, qt.IsFalse)
					})
				})
			})
		})
	}
}
