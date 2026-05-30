package testdb

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"codeberg.org/mna/karbur/errors"
	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/georgysavva/scany/v2/sqlscan"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type mocktb struct{ testing.TB }

func (tb *mocktb) Fatal(args ...any) {
	if len(args) > 0 {
		panic(args[0])
	}
	panic("no argument provided")
}

func requirePanicsMatching(t *testing.T, pattern string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic")
		var msg string
		switch v := r.(type) {
		case error:
			msg = v.Error()
		case string:
			msg = v
		default:
			msg = fmt.Sprint(v)
		}
		require.Regexp(t, pattern, msg)
	}()
	fn()
}

func TestNewPgx(t *testing.T) {
	// open a connection to the main (non-test) database
	conn, err := pgx.Connect(ctx, "")
	require.NoError(t, err)
	defer conn.Close(ctx) //nolint

	t.Run("fails with invalid connection string", func(t *testing.T) {
		mtb := &mocktb{t}
		requirePanicsMatching(t, `(?ms).+\b28P01\b.+`, func() {
			NewPgx(mtb, "user=nosuchuser password=clearlynot", "")
		})
	})

	t.Run("with test db", func(t *testing.T) {
		var testDBName string
		t.Cleanup(func() {
			// after cleanup, test database is removed (unless requested not to)
			var exists bool
			err := pgxscan.Get(ctx, conn, &exists, "SELECT true FROM pg_database WHERE datname = $1", testDBName)
			if os.Getenv("PGDB_KEEP_TESTDB") == "" {
				require.Error(t, err)
				require.False(t, exists)
				require.True(t, pgxscan.NotFound(err), "want not found, got %v", err)
			} else {
				require.NoError(t, err)
				require.True(t, exists)
			}
		})

		db := NewPgx(t, "", "testdb", MockPgcronPgx)
		require.NotNil(t, db)

		t.Run("creates database with prefix", func(t *testing.T) {
			err := pgxscan.Get(ctx, db, &testDBName, "SELECT current_database()")
			require.NoError(t, err)
			require.Regexp(t, `^testdb.+`, testDBName)
		})

		t.Run("creates database with test user", func(t *testing.T) {
			var user string
			err := pgxscan.Get(ctx, db, &user, "SELECT current_user")
			require.NoError(t, err)
			require.Regexp(t, `^testdb.+`, user)
		})

		t.Run("creates database with mocked pg_cron", func(t *testing.T) {
			var id int
			err := pgxscan.Get(ctx, db, &id, "SELECT cron.schedule('test-job', '* * * * *', 'SELECT 1')")
			require.NoError(t, err)
			require.True(t, id > 0)
		})
	})

	t.Run("with failing setup func", func(t *testing.T) {
		mtb := &mocktb{t}
		requirePanicsMatching(t, "nope", func() {
			NewPgx(mtb, "", "testdb", func(c *pgx.Conn) error {
				return errors.New("nope")
			})
		})
	})
}

func TestNewSQL(t *testing.T) {
	// open a connection to the main (non-test) database
	conn, err := pgx.Connect(ctx, "")
	require.NoError(t, err)
	defer conn.Close(ctx) //nolint

	t.Run("fails with invalid connection string", func(t *testing.T) {
		mtb := &mocktb{t}
		requirePanicsMatching(t, `(?ms).+\b28P01\b.+`, func() {
			NewSQL(mtb, "user=nosuchuser password=clearlynot", "")
		})
	})

	t.Run("with test db", func(t *testing.T) {
		var testDBName string
		t.Cleanup(func() {
			// after cleanup, test database is removed
			var exists bool
			err := pgxscan.Get(ctx, conn, &exists, "SELECT true FROM pg_database WHERE datname = $1", testDBName)
			if os.Getenv("PGDB_KEEP_TESTDB") == "" {
				require.Error(t, err)
				require.False(t, exists)
				require.True(t, pgxscan.NotFound(err), "want not found, got %v", err)
			} else {
				require.NoError(t, err)
				require.True(t, exists)
			}
		})

		db := NewSQL(t, "", "testdb", MockPgcronSQL)
		require.NotNil(t, db)

		t.Run("creates database with prefix", func(t *testing.T) {
			err := sqlscan.Get(ctx, db, &testDBName, "SELECT current_database()")
			require.NoError(t, err)
			require.Regexp(t, `^testdb.+`, testDBName)
		})

		t.Run("creates database with test user", func(t *testing.T) {
			var user string
			err := sqlscan.Get(ctx, db, &user, "SELECT current_user")
			require.NoError(t, err)
			require.Regexp(t, `^testdb.+`, user)
		})

		t.Run("creates database with mocked pg_cron", func(t *testing.T) {
			var id int
			err := sqlscan.Get(ctx, db, &id, "SELECT cron.schedule('test-job', '* * * * *', 'SELECT 1')")
			require.NoError(t, err)
			require.True(t, id > 0)
		})
	})

	t.Run("with failing setup func", func(t *testing.T) {
		mtb := &mocktb{t}
		requirePanicsMatching(t, "nope", func() {
			NewSQL(mtb, "", "testdb", func(c *sql.Conn) error {
				return errors.New("nope")
			})
		})
	})
}

func TestNewPqSQL(t *testing.T) {
	// open a connection to the main (non-test) database
	conn, err := pgx.Connect(ctx, "")
	require.NoError(t, err)
	defer conn.Close(ctx) //nolint

	t.Run("fails with invalid connection string", func(t *testing.T) {
		mtb := &mocktb{t}
		requirePanicsMatching(t, `(?ms).+\b28P01\b.+`, func() {
			NewPqSQL(mtb, "user=nosuchuser password=clearlynot", "")
		})
	})

	t.Run("with test db", func(t *testing.T) {
		var testDBName string
		t.Cleanup(func() {
			// after cleanup, test database is removed
			var exists bool
			err := pgxscan.Get(ctx, conn, &exists, "SELECT true FROM pg_database WHERE datname = $1", testDBName)
			if os.Getenv("PGDB_KEEP_TESTDB") == "" {
				require.Error(t, err)
				require.False(t, exists)
				require.True(t, pgxscan.NotFound(err), "want not found, got %v", err)
			} else {
				require.NoError(t, err)
				require.True(t, exists)
			}
		})

		db := NewPqSQL(t, "", "testdb", MockPgcronSQL)
		require.NotNil(t, db)

		t.Run("creates database with prefix", func(t *testing.T) {
			err := sqlscan.Get(ctx, db, &testDBName, "SELECT current_database()")
			require.NoError(t, err)
			require.Regexp(t, `^testdb.+`, testDBName)
		})

		t.Run("creates database with test user", func(t *testing.T) {
			var user string
			err := sqlscan.Get(ctx, db, &user, "SELECT current_user")
			require.NoError(t, err)
			require.Regexp(t, `^testdb.+`, user)
		})

		t.Run("creates database with mocked pg_cron", func(t *testing.T) {
			var id int
			err := sqlscan.Get(ctx, db, &id, "SELECT cron.schedule('test-job', '* * * * *', 'SELECT 1')")
			require.NoError(t, err)
			require.True(t, id > 0)
		})
	})

	t.Run("with failing setup func", func(t *testing.T) {
		mtb := &mocktb{t}
		requirePanicsMatching(t, "nope", func() {
			NewPqSQL(mtb, "", "testdb", func(c *sql.Conn) error {
				return errors.New("nope")
			})
		})
	})
}
