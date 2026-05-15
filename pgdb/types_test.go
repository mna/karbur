package pgdb_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/mna/karbur/pgdb"
	"github.com/mna/karbur/pgdb/pgxadapt"
	"github.com/mna/karbur/pgdb/sqladapt"
	"github.com/mna/karbur/pgdb/testdb"
)

// Tests marshaling of types to database types, handling of nulls, varchar
// limits, etc. across the supported drivers.
func TestTypes(t *testing.T) {
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

			_, err := pool.Exec(ctx, `
        CREATE TYPE status AS ENUM ('s1', 's2', 's3')
      `)
			c.Assert(err, qt.IsNil)

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
			c.Assert(err, qt.IsNil)

			c.Run("VarcharExceeds", func(c *qt.C) {
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, varchar_col) VALUES ('varchar_exceeds', $1)", "abcdefghijk")
				c.Assert(err, qt.IsNotNil)
				c.Assert(err.Error(), qt.Contains, "22001") // data too long
			})

			c.Run("SmallintOverflow", func(c *qt.C) {
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, smallint_col) VALUES ('smallint_overflow', $1)", 32768)
				c.Assert(err, qt.IsNotNil)
				c.Assert(err.Error(), qt.Contains, "greater than maximum value")
			})

			c.Run("JSONNil", func(c *qt.C) {
				var b []byte
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, jsonb_col) VALUES ('json_nil', $1)", b)
				c.Assert(err, qt.IsNil)
				err = pool.QueryOne(ctx, &b, "SELECT jsonb_col FROM ts WHERE ident = $1", "json_nil")
				c.Assert(err, qt.IsNil)
				c.Assert(b, qt.IsNil)
			})

			c.Run("JSONStructPtr", func(c *qt.C) {
				if tc.name == "sql" {
					c.Skipf("JSON marshaling not supported in driver %s", tc.name)
				}

				type j struct{ X int }
				type jcol struct {
					JsonbCol j
				}
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, jsonb_col) VALUES ('json_struct_ptr', $1)", &j{X: 1})
				c.Assert(err, qt.IsNil)

				var jout jcol
				err = pool.QueryOne(ctx, &jout, "SELECT jsonb_col FROM ts WHERE ident = $1", "json_struct_ptr")
				c.Assert(err, qt.IsNil)
				c.Assert(jout.JsonbCol, qt.Equals, j{X: 1})
			})

			c.Run("JSONMapNil", func(c *qt.C) {
				if tc.name == "sql" {
					c.Skipf("JSON marshaling not supported in driver %s", tc.name)
				}

				var m map[string]string
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, jsonb_col) VALUES ('json_map_nil', $1)", m)
				c.Assert(err, qt.IsNil)

				type jcol struct {
					JsonbCol map[string]string
				}
				var out jcol
				err = pool.QueryOne(ctx, &out, "SELECT jsonb_col FROM ts WHERE ident = $1", "json_map_nil")
				c.Assert(err, qt.IsNil)
				c.Assert(out.JsonbCol, qt.IsNil)
			})

			c.Run("IntPtrNil", func(c *qt.C) {
				var i *int
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, int_col) VALUES ('int_ptr_nil', $1)", i)
				c.Assert(err, qt.IsNil)

				err = pool.QueryOne(ctx, &i, "SELECT int_col FROM ts WHERE ident = $1", "int_ptr_nil")
				c.Assert(err, qt.IsNil)
				c.Assert(i, qt.IsNil)
			})

			c.Run("IntPtr", func(c *qt.C) {
				i := new(int)
				*i = 1
				_, err := pool.Exec(ctx, "INSERT INTO ts (ident, int_col) VALUES ('int_ptr', $1)", i)
				c.Assert(err, qt.IsNil)

				*i = 0
				err = pool.QueryOne(ctx, &i, "SELECT int_col FROM ts WHERE ident = $1", "int_ptr")
				c.Assert(err, qt.IsNil)
				c.Assert(i, qt.IsNotNil)
				c.Assert(*i, qt.Equals, 1)
			})
		})
	}
}
