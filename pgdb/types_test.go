package pgdb_test

import (
	"testing"

	"github.com/mna/karbur/pgdb"
	"github.com/mna/karbur/pgdb/pgxadapt"
	"github.com/mna/karbur/pgdb/sqladapt"
	"github.com/mna/karbur/pgdb/testdb"
	"github.com/stretchr/testify/require"
)

// Tests marshaling of types to database types, handling of nulls, varchar
// limits, etc. across the supported drivers.
func TestTypes(t *testing.T) {
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

			_, err := pool.Exec(ctx, `
        CREATE TYPE status AS ENUM ('s1', 's2', 's3')
      `)
			require.NoError(t, err)

			_, err = pool.Exec(ctx, `
        CREATE TABLE ts (
          ident         TEXT UNIQUE,
          smallint_col  SMALLINT,
          int_col       INTEGER,
          timestamp_col TIMESTAMPTZ,
          varchar_col   VARCHAR(10),
          jsonb_col     JSONB,
          enum_col      status
        )
      `)
			require.NoError(t, err)

			t.Run("VarcharExceeds", func(t *testing.T) {
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, varchar_col) VALUES ('varchar_exceeds', $1)", "abcdefghijk")
				require.Error(t, err)
				require.Contains(t, err.Error(), "22001") // data too long
			})

			t.Run("SmallintOverflow", func(t *testing.T) {
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, smallint_col) VALUES ('smallint_overflow', $1)", 32768)
				require.Error(t, err)
				require.Contains(t, err.Error(), "greater than maximum value")
			})

			t.Run("JSONNil", func(t *testing.T) {
				var b []byte
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, jsonb_col) VALUES ('json_nil', $1)", b)
				require.NoError(t, err)
				err = pool.QueryOne(ctx, &b, "SELECT jsonb_col FROM ts WHERE ident = $1", "json_nil")
				require.NoError(t, err)
				require.Nil(t, b)
			})

			t.Run("JSONStructPtr", func(t *testing.T) {
				if tc.name == "sql" {
					t.Skipf("JSON marshaling not supported in driver %s", tc.name)
				}

				type j struct{ X int }
				type jcol struct {
					JsonbCol j
				}
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, jsonb_col) VALUES ('json_struct_ptr', $1)", &j{X: 1})
				require.NoError(t, err)

				var jout jcol
				err = pool.QueryOne(ctx, &jout, "SELECT jsonb_col FROM ts WHERE ident = $1", "json_struct_ptr")
				require.NoError(t, err)
				require.Equal(t, j{X: 1}, jout.JsonbCol)
			})

			t.Run("JSONMapNil", func(t *testing.T) {
				if tc.name == "sql" {
					t.Skipf("JSON marshaling not supported in driver %s", tc.name)
				}

				var m map[string]string
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, jsonb_col) VALUES ('json_map_nil', $1)", m)
				require.NoError(t, err)

				type jcol struct {
					JsonbCol map[string]string
				}
				var out jcol
				err = pool.QueryOne(ctx, &out, "SELECT jsonb_col FROM ts WHERE ident = $1", "json_map_nil")
				require.NoError(t, err)
				require.Nil(t, out.JsonbCol)
			})

			t.Run("IntPtrNil", func(t *testing.T) {
				var i *int
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, int_col) VALUES ('int_ptr_nil', $1)", i)
				require.NoError(t, err)

				err = pool.QueryOne(ctx, &i, "SELECT int_col FROM ts WHERE ident = $1", "int_ptr_nil")
				require.NoError(t, err)
				require.Nil(t, i)
			})

			t.Run("IntPtr", func(t *testing.T) {
				i := new(int)
				*i = 1
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, int_col) VALUES ('int_ptr', $1)", i)
				require.NoError(t, err)

				*i = 0
				err = pool.QueryOne(ctx, &i, "SELECT int_col FROM ts WHERE ident = $1", "int_ptr")
				require.NoError(t, err)
				require.NotNil(t, i)
				require.Equal(t, 1, *i)
			})
		})
	}
}
