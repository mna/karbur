package testdb

import (
	"database/sql"
	"errors"
	"os"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/georgysavva/scany/v2/sqlscan"
	"github.com/jackc/pgx/v5"
)

type mocktb struct{ testing.TB }

func (tb *mocktb) Fatal(args ...interface{}) {
	if len(args) > 0 {
		panic(args[0])
	}
	panic("no argument provided")
}

func TestNewPgx(t *testing.T) {
	c := qt.New(t)

	// open a connection to the main (non-test) database
	conn, err := pgx.Connect(ctx, "")
	c.Assert(err, qt.IsNil)
	defer conn.Close(ctx)

	c.Run("fails with invalid connection string", func(c *qt.C) {
		mtb := &mocktb{c}
		c.Assert(func() { NewPgx(mtb, "user=nosuchuser password=clearlynot", "") }, qt.PanicMatches, `(?ms).+\b28P01\b.+`)
	})

	c.Run("with test db", func(c *qt.C) {
		var testDBName string
		c.Cleanup(func() {
			// after cleanup, test database is removed (unless requested not to)
			var exists bool
			err := pgxscan.Get(ctx, conn, &exists, "SELECT true FROM pg_database WHERE datname = $1", testDBName)
			if os.Getenv("PGDB_KEEP_TESTDB") == "" {
				c.Assert(err, qt.IsNotNil)
				c.Assert(exists, qt.IsFalse)
				c.Assert(pgxscan.NotFound(err), qt.IsTrue, qt.Commentf("want not found, got %v", err))
			} else {
				c.Assert(err, qt.IsNil)
				c.Assert(exists, qt.IsTrue)
			}
		})

		db := NewPgx(c, "", "testdb", MockPgcronPgx)
		c.Assert(db, qt.IsNotNil)

		c.Run("creates database with prefix", func(c *qt.C) {
			err := pgxscan.Get(ctx, db, &testDBName, "SELECT current_database()")
			c.Assert(err, qt.IsNil)
			c.Assert(testDBName, qt.Matches, `^testdb.+`)
		})

		c.Run("creates database with test user", func(c *qt.C) {
			var user string
			err := pgxscan.Get(ctx, db, &user, "SELECT current_user")
			c.Assert(err, qt.IsNil)
			c.Assert(user, qt.Matches, `^testdb.+`)
		})

		c.Run("creates database with mocked pg_cron", func(c *qt.C) {
			var id int
			err := pgxscan.Get(ctx, db, &id, "SELECT cron.schedule('test-job', '* * * * *', 'SELECT 1')")
			c.Assert(err, qt.IsNil)
			c.Assert(id > 0, qt.IsTrue)
		})
	})

	c.Run("with failing setup func", func(c *qt.C) {
		mtb := &mocktb{c}
		c.Assert(func() {
			NewPgx(mtb, "", "testdb", func(c *pgx.Conn) error {
				return errors.New("nope")
			})
		}, qt.PanicMatches, "nope")
	})
}

func TestNewSQL(t *testing.T) {
	c := qt.New(t)

	// open a connection to the main (non-test) database
	conn, err := pgx.Connect(ctx, "")
	c.Assert(err, qt.IsNil)
	defer conn.Close(ctx)

	c.Run("fails with invalid connection string", func(c *qt.C) {
		mtb := &mocktb{c}
		c.Assert(func() { NewSQL(mtb, "user=nosuchuser password=clearlynot", "") }, qt.PanicMatches, `(?ms).+\b28P01\b.+`)
	})

	c.Run("with test db", func(c *qt.C) {
		var testDBName string
		c.Cleanup(func() {
			// after cleanup, test database is removed
			var exists bool
			err := pgxscan.Get(ctx, conn, &exists, "SELECT true FROM pg_database WHERE datname = $1", testDBName)
			if os.Getenv("PGDB_KEEP_TESTDB") == "" {
				c.Assert(err, qt.IsNotNil)
				c.Assert(exists, qt.IsFalse)
				c.Assert(pgxscan.NotFound(err), qt.IsTrue, qt.Commentf("want not found, got %v", err))
			} else {
				c.Assert(err, qt.IsNil)
				c.Assert(exists, qt.IsTrue)
			}
		})

		db := NewSQL(c, "", "testdb", MockPgcronSQL)
		c.Assert(db, qt.IsNotNil)

		c.Run("creates database with prefix", func(c *qt.C) {
			err := sqlscan.Get(ctx, db, &testDBName, "SELECT current_database()")
			c.Assert(err, qt.IsNil)
			c.Assert(testDBName, qt.Matches, `^testdb.+`)
		})

		c.Run("creates database with test user", func(c *qt.C) {
			var user string
			err := sqlscan.Get(ctx, db, &user, "SELECT current_user")
			c.Assert(err, qt.IsNil)
			c.Assert(user, qt.Matches, `^testdb.+`)
		})

		c.Run("creates database with mocked pg_cron", func(c *qt.C) {
			var id int
			err := sqlscan.Get(ctx, db, &id, "SELECT cron.schedule('test-job', '* * * * *', 'SELECT 1')")
			c.Assert(err, qt.IsNil)
			c.Assert(id > 0, qt.IsTrue)
		})
	})

	c.Run("with failing setup func", func(c *qt.C) {
		mtb := &mocktb{c}
		c.Assert(func() {
			NewSQL(mtb, "", "testdb", func(c *sql.Conn) error {
				return errors.New("nope")
			})
		}, qt.PanicMatches, "nope")
	})
}
