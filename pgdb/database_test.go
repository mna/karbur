package pgdb_test

import (
	"context"
	"database/sql"
	"io"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"codeberg.org/mna/karbur/errors"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/pgxadapt"
	"codeberg.org/mna/karbur/pgdb/sqladapt"
	"codeberg.org/mna/karbur/pgdb/testdb"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func TestPool(t *testing.T) {
	cases := []struct {
		name  string
		setup func() pgdb.Pool
	}{
		{"pgx", func() pgdb.Pool { db := testdb.NewPgx(t, "", ""); return pgxadapt.ToPool(db) }},
		{"sql", func() pgdb.Pool { db := testdb.NewSQL(t, "", ""); return sqladapt.ToPool(db) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := tc.setup()

			t.Cleanup(func() {
				err := pool.Close()
				require.NoError(t, err)
			})

			t.Run("QueryOne", func(t *testing.T) {
				var name string
				err := pool.QueryOne(ctx, &name, "SELECT current_database()")
				require.NoError(t, err)
				require.Contains(t, name, "test")
			})

			t.Run("ExecFail", func(t *testing.T) {
				_, err := pool.Exec(ctx, "CREATE FOO bar")
				require.Error(t, err)
				require.Contains(t, err.Error(), "42601")
			})

			t.Run("Pool", func(t *testing.T) {
				res, err := pool.Exec(ctx, "CREATE TABLE testint (v integer primary key)")
				require.NoError(t, err)
				t.Cleanup(func() {
					_, _ = pool.Exec(ctx, "DROP TABLE testint")
				})

				n, err := res.RowsAffected()
				require.NoError(t, err)
				require.Equal(t, int64(0), n)
				_, err = res.LastInsertId()
				require.Error(t, err)

				for i := 1; i <= 5; i++ {
					res, err := pool.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", i)
					require.NoError(t, err)
					n, err := res.RowsAffected()
					require.NoError(t, err)
					require.Equal(t, int64(1), n)
				}

				t.Run("As", func(t *testing.T) {
					pgpool, sqlpool := new(pgxpool.Pool), new(sql.DB)
					pgok, sqlok := pool.As(&pgpool), pool.As(&sqlpool)
					require.NotEqual(t, sqlok, pgok)

					if pgok {
						err := pgpool.Ping(ctx)
						require.NoError(t, err)
					} else {
						err := sqlpool.PingContext(ctx)
						require.NoError(t, err)
					}
				})

				t.Run("QueryOneStruc", func(t *testing.T) {
					var dst struct{ V int }
					err := pool.QueryOne(ctx, &dst, "SELECT v FROM testint WHERE v = $1", 2)
					require.NoError(t, err)
					require.Equal(t, struct{ V int }{2}, dst)
				})

				t.Run("QueryOneNoRow", func(t *testing.T) {
					var dst int
					err := pool.QueryOne(ctx, &dst, "SELECT v FROM testint WHERE v = $1", -1)
					require.Error(t, err)
					require.True(t, errors.Is(err, sql.ErrNoRows))
				})

				t.Run("QueryMany", func(t *testing.T) {
					var ids []int
					err := pool.QueryMany(ctx, &ids, "SELECT v FROM testint")
					require.NoError(t, err)
					require.Equal(t, []int{1, 2, 3, 4, 5}, ids)
				})

				t.Run("Cursor", func(t *testing.T) {
					cur := pool.Cursor(ctx, "SELECT v FROM testint")
					defer cur.Close()

					var ids []int
					for cur.Next() {
						var id int
						err := cur.Scan(&id)
						require.NoError(t, err)
						ids = append(ids, id)
					}
					err := cur.Err()
					require.NoError(t, err)
					require.Equal(t, []int{1, 2, 3, 4, 5}, ids)
				})
			})

			t.Run("Txer", func(t *testing.T) {
				_, err := pool.Exec(ctx, "CREATE TABLE testint (v integer primary key)")
				require.NoError(t, err)
				t.Cleanup(func() {
					_, _ = pool.Exec(ctx, "DROP TABLE testint")
				})

				for i := 1; i <= 5; i++ {
					res, err := pool.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", i)
					require.NoError(t, err)
					n, err := res.RowsAffected()
					require.NoError(t, err)
					require.Equal(t, int64(1), n)
				}

				t.Run("BeginCommit", func(t *testing.T) {
					tx, err := pool.BeginTx(ctx, nil)
					require.NoError(t, err)
					defer func() { _ = tx.Rollback(ctx) }()

					res, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 6)
					require.NoError(t, err)

					n, err := res.RowsAffected()
					require.NoError(t, err)
					require.Equal(t, int64(1), n)
					_, err = res.LastInsertId()
					require.Error(t, err)

					err = tx.Commit(ctx)
					require.NoError(t, err)

					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 6)
					require.NoError(t, err)
					require.True(t, ok)
				})

				t.Run("BeginRollback", func(t *testing.T) {
					tx, err := pool.BeginTx(ctx, nil)
					require.NoError(t, err)
					defer func() { _ = tx.Rollback(ctx) }()

					_, err = tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 7)
					require.NoError(t, err)

					err = tx.Rollback(ctx)
					require.NoError(t, err)

					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 7)
					require.Error(t, err)
					require.True(t, errors.Is(err, sql.ErrNoRows))
					require.False(t, ok)
				})

				t.Run("BeginFuncCommit", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 8)
						require.NoError(t, err)
						return nil
					})
					require.NoError(t, err)

					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 8)
					require.NoError(t, err)
					require.True(t, ok)
				})

				t.Run("BeginFuncRollback", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 9)
						require.NoError(t, err)
						return errors.New("nope")
					})
					require.Error(t, err)
					require.Contains(t, err.Error(), "nope")

					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 9)
					require.Error(t, err)
					require.True(t, errors.Is(err, sql.ErrNoRows))
					require.False(t, ok)
				})

				t.Run("TxerAs", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						var pgtx pgx.Tx
						sqltx := new(sql.Tx)
						pgok, sqlok := tx.As(&pgtx), tx.As(&sqltx)
						require.NotEqual(t, sqlok, pgok)

						var i int
						if pgok {
							row := pgtx.QueryRow(ctx, "SELECT 1")
							return row.Scan(&i)
						}
						row := sqltx.QueryRow("SELECT 1")
						return row.Scan(&i)
					})
					require.NoError(t, err)
				})

				t.Run("TxerQueryOne", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 10)
						require.NoError(t, err)

						var ok bool
						err = tx.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 10)
						require.NoError(t, err)
						require.True(t, ok)
						return nil
					})
					require.NoError(t, err)
				})

				t.Run("TxerQueryOneNoRows", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						var ok bool
						err := tx.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", -1)
						require.Error(t, err)
						require.False(t, ok)
						return err
					})
					require.Error(t, err)
					require.True(t, errors.Is(err, sql.ErrNoRows))
				})

				t.Run("TxerQueryMany", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						var ids []int
						err := tx.QueryMany(ctx, &ids, "SELECT v FROM testint WHERE v < $1", 5)
						require.NoError(t, err)
						require.Equal(t, []int{1, 2, 3, 4}, ids)
						return nil
					})
					require.NoError(t, err)
				})

				t.Run("TxerCursor", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						var ids []int
						cur := tx.Cursor(ctx, "SELECT v FROM testint WHERE v < $1", 5)
						for cur.Next() {
							var id int
							err := cur.Scan(&id)
							require.NoError(t, err)
							ids = append(ids, id)
						}
						require.Equal(t, []int{1, 2, 3, 4}, ids)
						return cur.Err()
					})
					require.NoError(t, err)
				})

				t.Run("TxerExecFail", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						_, err := tx.Exec(ctx, "INSERT INTO no_Such_TABLE (v) VALUES ($1)", 1)
						require.Error(t, err)
						return err
					})
					require.Error(t, err)
					require.Contains(t, err.Error(), "42P01")
				})

				t.Run("EnsureTxNone", func(t *testing.T) {
					err := pgdb.EnsureTx(ctx, pool, func(ctx context.Context, tx pgdb.Txer) error {
						_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 11)
						require.NoError(t, err)
						return nil
					})
					require.NoError(t, err)

					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 11)
					require.NoError(t, err)
					require.True(t, ok)
				})

				t.Run("EnsureTxExistCommit", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						err := pgdb.EnsureTx(ctx, pool, func(ctx context.Context, tx pgdb.Txer) error {
							_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 12)
							require.NoError(t, err)
							return nil
						})
						require.NoError(t, err)

						// does not exist yet outside the transaction
						var ok bool
						err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 12)
						require.True(t, errors.Is(err, sql.ErrNoRows))
						require.False(t, ok)

						return nil
					})
					require.NoError(t, err)

					// exists now, after commit
					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 12)
					require.NoError(t, err)
					require.True(t, ok)
				})

				t.Run("EnsureTxExistRollback", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						err := pgdb.EnsureTx(ctx, pool, func(ctx context.Context, tx pgdb.Txer) error {
							_, err := tx.Exec(ctx, "INSERT INTO no_SUCH_Table (v) VALUES ($1)", 1)
							require.Error(t, err)
							return err
						})
						require.Error(t, err)
						require.Contains(t, err.Error(), "42P01")
						return err
					})
					require.Error(t, err)
				})

				t.Run("RequireTxNone", func(t *testing.T) {
					err := pgdb.RequireTx(ctx, func(ctx context.Context, tx pgdb.Txer) error {
						panic("should not be called")
					})
					require.Error(t, err)
					require.True(t, errors.Is(err, pgdb.ErrNoTx))
				})

				t.Run("RequireTxExistCommit", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						err := pgdb.RequireTx(ctx, func(ctx context.Context, tx pgdb.Txer) error {
							_, err := tx.Exec(ctx, "INSERT INTO testint (v) VALUES ($1)", 13)
							require.NoError(t, err)
							return nil
						})
						require.NoError(t, err)

						// does not exist yet outside the transaction
						var ok bool
						err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 13)
						require.True(t, errors.Is(err, sql.ErrNoRows))
						require.False(t, ok)

						return nil
					})
					require.NoError(t, err)

					// exists now, after commit
					var ok bool
					err = pool.QueryOne(ctx, &ok, "SELECT true FROM testint WHERE v = $1", 13)
					require.NoError(t, err)
					require.True(t, ok)
				})

				t.Run("RequireTxExistRollback", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						err := pgdb.RequireTx(ctx, func(ctx context.Context, tx pgdb.Txer) error {
							_, err := tx.Exec(ctx, "INSERT INTO no_SUCH_Table (v) VALUES ($1)", 1)
							require.Error(t, err)
							return err
						})
						require.Error(t, err)
						require.Contains(t, err.Error(), "42P01")
						return err
					})
					require.Error(t, err)
				})

				t.Run("EnsureQueryerNone", func(t *testing.T) {
					err := pgdb.EnsureQueryer(ctx, pool, func(ctx context.Context, q pgdb.Queryer) error {
						var count int
						err := pool.QueryOne(ctx, &count, "SELECT count(*) FROM testint")
						require.NoError(t, err)
						require.True(t, count >= 5)
						return nil
					})
					require.NoError(t, err)
				})

				t.Run("EnsureQueryerExist", func(t *testing.T) {
					err := pgdb.Tx(ctx, pool, nil, func(ctx context.Context, tx pgdb.Txer) error {
						err := pgdb.EnsureQueryer(ctx, pool, func(ctx context.Context, q pgdb.Queryer) error {
							var count int
							err := pool.QueryOne(ctx, &count, "SELECT count(*) FROM testint")
							require.NoError(t, err)
							require.True(t, count >= 5)
							return io.EOF
						})
						require.True(t, errors.Is(err, io.EOF))
						return err
					})
					require.True(t, errors.Is(err, io.EOF))
				})
			})

			t.Run("Conn", func(t *testing.T) {
				conn, err := pool.Conn(ctx)
				require.NoError(t, err)
				t.Cleanup(func() {
					err := conn.Close()
					require.NoError(t, err)
				})

				t.Run("As", func(t *testing.T) {
					pgconn, sqlconn := new(pgxpool.Conn), new(sql.Conn)
					pgok, sqlok := conn.As(&pgconn), conn.As(&sqlconn)
					require.NotEqual(t, sqlok, pgok)

					if pgok {
						err := pgconn.Ping(ctx)
						require.NoError(t, err)
					} else {
						err := sqlconn.PingContext(ctx)
						require.NoError(t, err)
					}
				})

				t.Run("Exec", func(t *testing.T) {
					res, err := conn.Exec(ctx, "CREATE TABLE testint_conn (v integer primary key)")
					require.NoError(t, err)

					n, err := res.RowsAffected()
					require.NoError(t, err)
					require.Equal(t, int64(0), n)
					_, err = res.LastInsertId()
					require.Error(t, err)

					for i := 1; i <= 5; i++ {
						res, err := conn.Exec(ctx, "INSERT INTO testint_conn (v) VALUES ($1)", i)
						require.NoError(t, err)
						n, err := res.RowsAffected()
						require.NoError(t, err)
						require.Equal(t, int64(1), n)
					}

					t.Run("QueryOneStruc", func(t *testing.T) {
						var dst struct{ V int }
						err := conn.QueryOne(ctx, &dst, "SELECT v FROM testint_conn WHERE v = $1", 2)
						require.NoError(t, err)
						require.Equal(t, struct{ V int }{2}, dst)
					})

					t.Run("QueryOneNoRow", func(t *testing.T) {
						var dst int
						err := conn.QueryOne(ctx, &dst, "SELECT v FROM testint_conn WHERE v = $1", -1)
						require.Error(t, err)
						require.True(t, errors.Is(err, sql.ErrNoRows))
					})

					t.Run("QueryMany", func(t *testing.T) {
						var ids []int
						err := conn.QueryMany(ctx, &ids, "SELECT v FROM testint_conn")
						require.NoError(t, err)
						require.Equal(t, []int{1, 2, 3, 4, 5}, ids)
					})

					t.Run("Cursor", func(t *testing.T) {
						cur := conn.Cursor(ctx, "SELECT v FROM testint_conn")
						defer cur.Close()

						var ids []int
						for cur.Next() {
							var id int
							err := cur.Scan(&id)
							require.NoError(t, err)
							ids = append(ids, id)
						}
						err := cur.Err()
						require.NoError(t, err)
						require.Equal(t, []int{1, 2, 3, 4, 5}, ids)
					})

					t.Run("BeginCommit", func(t *testing.T) {
						tx, err := conn.BeginTx(ctx, nil)
						require.NoError(t, err)
						defer func() { _ = tx.Rollback(ctx) }()

						res, err := tx.Exec(ctx, "INSERT INTO testint_conn (v) VALUES ($1)", 6)
						require.NoError(t, err)

						n, err := res.RowsAffected()
						require.NoError(t, err)
						require.Equal(t, int64(1), n)
						_, err = res.LastInsertId()
						require.Error(t, err)

						err = tx.Commit(ctx)
						require.NoError(t, err)

						var ok bool
						err = conn.QueryOne(ctx, &ok, "SELECT true FROM testint_conn WHERE v = $1", 6)
						require.NoError(t, err)
						require.True(t, ok)
					})

					t.Run("BeginRollback", func(t *testing.T) {
						tx, err := conn.BeginTx(ctx, nil)
						require.NoError(t, err)
						defer func() { _ = tx.Rollback(ctx) }()

						_, err = tx.Exec(ctx, "INSERT INTO testint_conn (v) VALUES ($1)", 7)
						require.NoError(t, err)

						err = tx.Rollback(ctx)
						require.NoError(t, err)

						var ok bool
						err = conn.QueryOne(ctx, &ok, "SELECT true FROM testint_conn WHERE v = $1", 7)
						require.Error(t, err)
						require.True(t, errors.Is(err, sql.ErrNoRows))
						require.False(t, ok)
					})
				})
			})
		})
	}
}
