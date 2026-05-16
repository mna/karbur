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

	"github.com/mna/karbur/errors"
	"github.com/mna/karbur/pgdb"
	"github.com/mna/karbur/pgdb/mockdb"
	"github.com/mna/karbur/pgdb/pgxadapt"
	"github.com/mna/karbur/pgdb/sqladapt"
	"github.com/mna/karbur/pgdb/testdb"
	"github.com/stretchr/testify/require"
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
	//go:embed testdata/groupconfig
	groupconfig embed.FS
)

func TestMigrator(t *testing.T) {
	// root the embedded FS
	groupaMigs, _ := fs.Sub(groupa, "testdata/groupa")
	groupbMigs, _ := fs.Sub(groupb, "testdata/groupb")
	groupcMigs, _ := fs.Sub(groupc, "testdata/groupc")
	emptyMigs, _ := fs.Sub(empty, "testdata/empty")
	failMigs, _ := fs.Sub(fail, "testdata/fail")
	groupconfigMigs, _ := fs.Sub(groupconfig, "testdata/groupconfig")

	t.Run("Export", func(t *testing.T) {
		dir := t.TempDir()
		mockPool := mockdb.NewPool()
		mig, err := New(mockPool, nil)
		require.NoError(t, err)

		err = mig.Register("a", nil, groupaMigs)
		require.NoError(t, err)
		err = mig.Register("b", nil, groupbMigs)
		require.NoError(t, err)

		err = mig.Export(dir)
		require.NoError(t, err)

		ents, err := os.ReadDir(dir)
		require.NoError(t, err)

		require.Equal(t, 2, len(ents))
		require.Equal(t, "a", ents[0].Name())
		require.True(t, ents[0].IsDir())
		require.Equal(t, "b", ents[1].Name())
		require.True(t, ents[1].IsDir())

		entsA, err := os.ReadDir(filepath.Join(dir, ents[0].Name()))
		require.NoError(t, err)
		require.Equal(t, 3, len(entsA))
		require.Equal(t, "001.sql.tpl", entsA[0].Name())
		require.Equal(t, "002.sql.tpl", entsA[1].Name())
		require.Equal(t, "003.sql.tpl", entsA[2].Name())

		entsB, err := os.ReadDir(filepath.Join(dir, ents[1].Name()))
		require.NoError(t, err)
		require.Equal(t, 1, len(entsB))
		require.Equal(t, "001.sql.tpl", entsB[0].Name())
	})

	t.Run("RegisterDuplicate", func(t *testing.T) {
		mig, err := New(mockdb.NewPool(), nil)
		require.NoError(t, err)
		err = mig.Register("a", nil, emptyMigs)
		require.NoError(t, err)
		err = mig.Register("a", nil, emptyMigs)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrGroupRegistered))
	})

	cases := []struct {
		name  string
		setup func() pgdb.Pool
	}{
		{"pgx", func() pgdb.Pool { db := testdb.NewPgx(t, "", ""); return pgxadapt.ToPool(db) }},
		{"sql", func() pgdb.Pool { db := testdb.NewSQL(t, "", ""); return sqladapt.ToPool(db) }},
	}
	for _, tc := range cases {
		t.Run("Migrate:"+tc.name, func(t *testing.T) {
			pool := tc.setup()

			drop := func() {
				if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS migrate_versions"); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS migrate_v"); err != nil {
					t.Fatal(err)
				}
			}

			t.Run("Register", func(t *testing.T) {
				t.Cleanup(drop)

				mig, err := New(pool, nil)
				require.NoError(t, err)
				err = mig.Register("a", nil, emptyMigs, "b", "c")
				require.NoError(t, err)
				err = mig.Register("b", nil, emptyMigs)
				require.NoError(t, err)
				err = mig.Register("c", nil, emptyMigs, "b")
				require.NoError(t, err)

				err = mig.Migrate(ctx)
				require.NoError(t, err)

				var version int
				err = pool.QueryOne(ctx, &version, "SELECT version FROM migrate_versions")
				require.Error(t, err)
				require.True(t, errors.Is(err, sql.ErrNoRows))
			})

			t.Run("MigrateOrder", func(t *testing.T) {
				t.Cleanup(drop)

				mig, err := New(pool, nil)
				require.NoError(t, err)
				err = mig.Register("a", nil, groupaMigs)
				require.NoError(t, err)

				err = mig.Register("b", nil, groupbMigs, "a", "c")
				require.NoError(t, err)

				err = mig.Register("c", nil, groupcMigs, "a")
				require.NoError(t, err)

				err = mig.Migrate(ctx)
				require.NoError(t, err)

				var vals []int
				err = pool.QueryMany(ctx, &vals, "SELECT v FROM migrate_v ORDER BY id")
				require.NoError(t, err)
				require.Equal(t, []int{1, 2, 4, 5, 6, 3}, vals)
			})

			t.Run("MigrateCycle", func(t *testing.T) {
				t.Cleanup(drop)

				mig, err := New(pool, nil)
				require.NoError(t, err)
				err = mig.Register("a", nil, emptyMigs, "b", "c")
				require.NoError(t, err)
				err = mig.Register("b", nil, emptyMigs, "c")
				require.NoError(t, err)
				err = mig.Register("c", nil, emptyMigs, "a")
				require.NoError(t, err)

				err = mig.Migrate(ctx)
				require.Error(t, err)
				require.True(t, errors.Is(err, ErrCycle))
			})

			t.Run("MigrateMissingDep", func(t *testing.T) {
				t.Cleanup(drop)

				mig, err := New(pool, nil)
				require.NoError(t, err)
				err = mig.Register("a", nil, emptyMigs, "b", "c")
				require.NoError(t, err)
				err = mig.Register("b", nil, emptyMigs, "d")
				require.NoError(t, err)

				err = mig.Migrate(ctx)
				require.Error(t, err)
				require.True(t, errors.Is(err, ErrMissingGroup))
				require.Contains(t, err.Error(), "[c d]")
			})

			t.Run("MigrateRollback", func(t *testing.T) {
				t.Cleanup(drop)

				mig, err := New(pool, nil)
				require.NoError(t, err)
				err = mig.Register("a", nil, groupaMigs)
				require.NoError(t, err)

				err = mig.Register("f", nil, failMigs, "a")
				require.NoError(t, err)

				err = mig.Migrate(ctx)
				require.Error(t, err)
				require.Contains(t, err.Error(), "42703") // invalid column

				var val int
				err = pool.QueryOne(ctx, &val, "SELECT v FROM migrate_v")
				require.Error(t, err)
				require.Contains(t, err.Error(), "42P01") // invalid table
			})

			t.Run("MigrateConcurrency", func(t *testing.T) {
				t.Cleanup(drop)

				migs := make([]*Migrator, 3)
				for i := range migs {
					mig, err := New(pool, nil)
					require.NoError(t, err)
					err = mig.Register("a", nil, groupaMigs)
					require.NoError(t, err)

					err = mig.Register("b", nil, groupbMigs, "a")
					require.NoError(t, err)

					migs[i] = mig
				}

				var wg sync.WaitGroup
				wg.Add(3)
				for i := 0; i < 3; i++ {
					go func(mig *Migrator) {
						defer wg.Done()
						err := mig.Migrate(ctx)
						require.NoError(t, err)
					}(migs[i])
				}
				wg.Wait()

				var vals []int
				err := pool.QueryMany(ctx, &vals, "SELECT v FROM migrate_v ORDER BY id")
				require.NoError(t, err)
				require.Equal(t, []int{1, 2, 3}, vals)
			})

			t.Run("MigrateWithConfig", func(t *testing.T) {
				t.Cleanup(drop)
				t.Cleanup(func() { _, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS migrate_conf") })

				type tplConfig struct{ V int }

				mig, err := New(pool, nil)
				require.NoError(t, err)
				err = mig.Register("cfg", tplConfig{V: 42}, groupconfigMigs)
				require.NoError(t, err)

				err = mig.Migrate(ctx)
				require.NoError(t, err)

				var v int
				err = pool.QueryOne(ctx, &v, "SELECT v FROM migrate_conf")
				require.NoError(t, err)
				require.Equal(t, 42, v)
			})

			t.Run("MigrateIdempotent", func(t *testing.T) {
				t.Cleanup(drop)

				mig, err := New(pool, nil)
				require.NoError(t, err)
				err = mig.Register("a", nil, groupaMigs)
				require.NoError(t, err)

				err = mig.Migrate(ctx)
				require.NoError(t, err)

				var first int
				err = pool.QueryOne(ctx, &first, "SELECT count(*) FROM migrate_v")
				require.NoError(t, err)

				err = mig.Migrate(ctx)
				require.NoError(t, err)

				var second int
				err = pool.QueryOne(ctx, &second, "SELECT count(*) FROM migrate_v")
				require.NoError(t, err)
				require.Equal(t, first, second)
			})

			t.Run("MigrateIncremental", func(t *testing.T) {
				t.Cleanup(drop)

				// First pass: groupa only (creates table, inserts 1, 2).
				mig1, err := New(pool, nil)
				require.NoError(t, err)
				err = mig1.Register("a", nil, groupaMigs)
				require.NoError(t, err)
				err = mig1.Migrate(ctx)
				require.NoError(t, err)

				var vals []int
				err = pool.QueryMany(ctx, &vals, "SELECT v FROM migrate_v ORDER BY id")
				require.NoError(t, err)
				require.Equal(t, []int{1, 2}, vals)

				// Second pass: add groupc + groupb on top of the already-applied groupa.
				mig2, err := New(pool, nil)
				require.NoError(t, err)
				err = mig2.Register("a", nil, groupaMigs)
				require.NoError(t, err)
				err = mig2.Register("c", nil, groupcMigs, "a")
				require.NoError(t, err)
				err = mig2.Register("b", nil, groupbMigs, "a", "c")
				require.NoError(t, err)
				err = mig2.Migrate(ctx)
				require.NoError(t, err)

				err = pool.QueryMany(ctx, &vals, "SELECT v FROM migrate_v ORDER BY id")
				require.NoError(t, err)
				require.Equal(t, []int{1, 2, 4, 5, 6, 3}, vals)
			})

			t.Run("MigrateAdvisoryLockID", func(t *testing.T) {
				t.Cleanup(drop)

				mig, err := New(pool, &Config{AdvisoryLockID: 42})
				require.NoError(t, err)
				err = mig.Register("a", nil, groupaMigs)
				require.NoError(t, err)
				err = mig.Migrate(ctx)
				require.NoError(t, err)

				var n int
				err = pool.QueryOne(ctx, &n, "SELECT count(*) FROM migrate_v")
				require.NoError(t, err)
				require.Equal(t, 2, n)
			})
		})
	}
}
