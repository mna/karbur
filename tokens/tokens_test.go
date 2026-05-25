package tokens

import (
	"context"
	"testing"
	"time"

	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/migrate"
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

			// apply the tokens' migrations
			mig, err := migrate.New(pool, nil)
			require.NoError(t, err)
			err = RegisterMigrations(mig)
			require.NoError(t, err)
			err = mig.Migrate(ctx)
			require.NoError(t, err)

			// create a token without a type
			_, err = New(ctx, pool, TokenArgs{
				Type:      "",
				RefID:     1,
				SingleUse: true,
				Expiry:    time.Second,
			})
			require.Error(t, err)
			require.ErrorContains(t, err, "SQLSTATE 23514") // violates check constraint

			// create a single-use token
			tok1, err := New(ctx, pool, TokenArgs{
				Type:      "test",
				RefID:     1,
				SingleUse: true,
				Expiry:    time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok1)

			// verify the token
			dbtok, err := Verify(ctx, pool, tok1, MustMatchTypeAndRefID("test", 1))
			require.NoError(t, err)
			require.Equal(t, tok1, dbtok.Token)
			require.WithinDuration(t, time.Now().Add(time.Minute), dbtok.Expiry, 2*time.Second)

			// verify the token again, now invalid
			_, err = Verify(ctx, pool, tok1, MustMatchTypeAndRefID("test", 1))
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
}
