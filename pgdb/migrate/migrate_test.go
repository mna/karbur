package migrate

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/mna/karbur/errors"
	"github.com/mna/karbur/pgdb"
	"github.com/mna/karbur/pgdb/mockdb"
	"github.com/mna/karbur/pgdb/pgxadapt"
	"github.com/mna/karbur/pgdb/sqladapt"
	"github.com/mna/karbur/pgdb/testdb"
)

var ctx = context.Background()

var (
	//go:embed testdata/groupa
	groupa embed.FS
	//go:embed testdata/groupb
	groupb embed.FS
	//go:embed testdata/groupc
	groupc embed.FS
	//go:embed testdata/empty
	empty embed.FS
	//go:embed testdata/fail
	fail embed.FS
)

func TestMigrator(t *testing.T) {
	c := qt.New(t)

	// root the embedded FS
	groupaMigs, _ := fs.Sub(groupa, "testdata/groupa")
	groupbMigs, _ := fs.Sub(groupb, "testdata/groupb")
	groupcMigs, _ := fs.Sub(groupc, "testdata/groupc")
	emptyMigs, _ := fs.Sub(empty, "testdata/empty")
	failMigs, _ := fs.Sub(fail, "testdata/fail")

	c.Run("Export", func(c *qt.C) {
		dir := c.TempDir()
		mockPool := mockdb.NewPool()
		mig, err := New(mockPool, nil)
		c.Assert(err, qt.IsNil)

		err = mig.Register("a", nil, groupaMigs)
		c.Assert(err, qt.IsNil)
		err = mig.Register("b", nil, groupbMigs)
		c.Assert(err, qt.IsNil)

		err = mig.Export(dir)
		c.Assert(err, qt.IsNil)

		ents, err := os.ReadDir(dir)
		c.Assert(err, qt.IsNil)

		c.Assert(len(ents), qt.Equals, 2)
		c.Assert(ents[0].Name(), qt.Equals, "a")
		c.Assert(ents[0].IsDir(), qt.IsTrue)
		c.Assert(ents[1].Name(), qt.Equals, "b")
		c.Assert(ents[1].IsDir(), qt.IsTrue)

		entsA, err := os.ReadDir(filepath.Join(dir, ents[0].Name()))
		c.Assert(err, qt.IsNil)
		c.Assert(len(entsA), qt.Equals, 3)
		c.Assert(entsA[0].Name(), qt.Equals, "001.sql.tpl")
		c.Assert(entsA[1].Name(), qt.Equals, "002.sql.tpl")
		c.Assert(entsA[2].Name(), qt.Equals, "003.sql.tpl")

		entsB, err := os.ReadDir(filepath.Join(dir, ents[1].Name()))
		c.Assert(err, qt.IsNil)
		c.Assert(len(entsB), qt.Equals, 1)
		c.Assert(entsB[0].Name(), qt.Equals, "001.sql.tpl")
	})

	cases := []struct {
		name  string
		setup func() pgdb.Pool
	}{
		{"pgx", func() pgdb.Pool { db := testdb.NewPgx(c, "", ""); return pgxadapt.ToPool(db) }},
		{"sql", func() pgdb.Pool { db := testdb.NewSQL(c, "", ""); return sqladapt.ToPool(db) }},
	}
	for _, tc := range cases {
		c.Run("Migrate:"+tc.name, func(c *qt.C) {
			pool := tc.setup()

			drop := func() {
				if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS migrate_versions"); err != nil {
					c.Fatal(err)
				}
				if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS migrate_v"); err != nil {
					c.Fatal(err)
				}
			}

			c.Run("Register", func(c *qt.C) {
				c.Cleanup(drop)

				mig, err := New(pool, nil)
				c.Assert(err, qt.IsNil)
				err = mig.Register("a", nil, emptyMigs, "b", "c")
				c.Assert(err, qt.IsNil)
				err = mig.Register("b", nil, emptyMigs)
				c.Assert(err, qt.IsNil)
				err = mig.Register("c", nil, emptyMigs, "b")
				c.Assert(err, qt.IsNil)

				err = mig.Migrate(ctx)
				c.Assert(err, qt.IsNil)

				var version int
				err = pool.QueryOne(ctx, &version, "SELECT version FROM migrate_versions")
				c.Assert(err, qt.IsNotNil)
				c.Assert(errors.Is(err, sql.ErrNoRows), qt.IsTrue)
			})

			c.Run("MigrateOrder", func(c *qt.C) {
				c.Cleanup(drop)

				mig, err := New(pool, nil)
				c.Assert(err, qt.IsNil)
				err = mig.Register("a", nil, groupaMigs)
				c.Assert(err, qt.IsNil)

				err = mig.Register("b", nil, groupbMigs, "a", "c")
				c.Assert(err, qt.IsNil)

				err = mig.Register("c", nil, groupcMigs, "a")
				c.Assert(err, qt.IsNil)

				err = mig.Migrate(ctx)
				c.Assert(err, qt.IsNil)

				var vals []int
				err = pool.QueryMany(ctx, &vals, "SELECT v FROM migrate_v ORDER BY id")
				c.Assert(err, qt.IsNil)
				c.Assert(vals, qt.DeepEquals, []int{1, 2, 4, 5, 6, 3})
			})

			c.Run("MigrateCycle", func(c *qt.C) {
				c.Cleanup(drop)

				mig, err := New(pool, nil)
				c.Assert(err, qt.IsNil)
				err = mig.Register("a", nil, emptyMigs, "b", "c")
				c.Assert(err, qt.IsNil)
				err = mig.Register("b", nil, emptyMigs, "c")
				c.Assert(err, qt.IsNil)
				err = mig.Register("c", nil, emptyMigs, "a")
				c.Assert(err, qt.IsNil)

				err = mig.Migrate(ctx)
				c.Assert(err, qt.IsNotNil)
				c.Assert(errors.Is(err, ErrCycle), qt.IsTrue)
			})

			c.Run("MigrateMissingDep", func(c *qt.C) {
				c.Cleanup(drop)

				mig, err := New(pool, nil)
				c.Assert(err, qt.IsNil)
				err = mig.Register("a", nil, emptyMigs, "b", "c")
				c.Assert(err, qt.IsNil)
				err = mig.Register("b", nil, emptyMigs, "d")
				c.Assert(err, qt.IsNil)

				err = mig.Migrate(ctx)
				c.Assert(err, qt.IsNotNil)
				c.Assert(errors.Is(err, ErrMissingGroup), qt.IsTrue)
				c.Assert(err.Error(), qt.Contains, "[c d]")
			})

			c.Run("MigrateRollback", func(c *qt.C) {
				c.Cleanup(drop)

				mig, err := New(pool, nil)
				c.Assert(err, qt.IsNil)
				err = mig.Register("a", nil, groupaMigs)
				c.Assert(err, qt.IsNil)

				err = mig.Register("f", nil, failMigs, "a")
				c.Assert(err, qt.IsNil)

				err = mig.Migrate(ctx)
				c.Assert(err, qt.IsNotNil)
				c.Assert(err.Error(), qt.Contains, "42703") // invalid column

				var val int
				err = pool.QueryOne(ctx, &val, "SELECT v FROM migrate_v")
				c.Assert(err, qt.IsNotNil)
				c.Assert(err.Error(), qt.Contains, "42P01") // invalid table
			})

			c.Run("MigrateConcurrency", func(c *qt.C) {
				c.Cleanup(drop)

				migs := make([]*Migrator, 3)
				for i := range migs {
					mig, err := New(pool, nil)
					c.Assert(err, qt.IsNil)
					err = mig.Register("a", nil, groupaMigs)
					c.Assert(err, qt.IsNil)

					err = mig.Register("b", nil, groupbMigs, "a")
					c.Assert(err, qt.IsNil)

					migs[i] = mig
				}

				var wg sync.WaitGroup
				wg.Add(3)
				for i := 0; i < 3; i++ {
					go func(mig *Migrator) {
						defer wg.Done()
						err := mig.Migrate(ctx)
						c.Assert(err, qt.IsNil)
					}(migs[i])
				}
				wg.Wait()

				var vals []int
				err := pool.QueryMany(ctx, &vals, "SELECT v FROM migrate_v ORDER BY id")
				c.Assert(err, qt.IsNil)
				c.Assert(vals, qt.DeepEquals, []int{1, 2, 3})
			})
		})
	}
}
