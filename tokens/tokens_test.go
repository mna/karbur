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

			// create another single-use token
			tok2a, err := New(ctx, pool, TokenArgs{
				Type:      "test",
				RefID:     2,
				SingleUse: true,
				Expiry:    time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok2a)

			// generate another for the same type/ref, will replace it
			tok2b, err := New(ctx, pool, TokenArgs{
				Type:      "test",
				RefID:     2,
				SingleUse: true,
				Expiry:    time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok2b)
			require.NotEqual(t, tok2a, tok2b)

			// verify the initial token, invalid
			_, err = Verify(ctx, pool, tok2a, nil)
			require.ErrorIs(t, err, ErrInvalid)

			// verify the new token, valid
			_, err = Verify(ctx, pool, tok2b, nil)
			require.NoError(t, err)

			// generate a multi-use token
			tok3, err := New(ctx, pool, TokenArgs{
				Type:      "test",
				RefID:     3,
				SingleUse: false,
				Expiry:    time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok3)

			// verify it, valid
			dbtok, err = Verify(ctx, pool, tok3, MustMatchType("test"))
			require.NoError(t, err)
			require.Equal(t, tok3, dbtok.Token)
			require.EqualValues(t, 3, dbtok.RefID)

			// verify it with a non-matching type, invalid
			_, err = Verify(ctx, pool, tok3, MustMatchType("NO-SUCH-TYPE"))
			require.ErrorIs(t, err, ErrInvalid)

			// verify it again, still valid
			dbtok, err = Verify(ctx, pool, tok3, MustMatchType("test"))
			require.NoError(t, err)
			require.EqualValues(t, 3, dbtok.RefID)

			// can create another multi-use for the same type/ref
			tok4, err := New(ctx, pool, TokenArgs{
				Type:      "test",
				RefID:     3,
				SingleUse: false,
				Expiry:    time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok4)
			require.NotEqual(t, tok3, tok4)

			// both are still valid
			dbtok, err = Verify(ctx, pool, tok3, MustMatchType("test"))
			require.NoError(t, err)
			require.EqualValues(t, 3, dbtok.RefID)
			dbtok, err = Verify(ctx, pool, tok4, MustMatchType("test"))
			require.NoError(t, err)
			require.EqualValues(t, 3, dbtok.RefID)

			// create a short-lived multi-use
			tok5, err := New(ctx, pool, TokenArgs{
				Type:      "test",
				RefID:     5,
				SingleUse: false,
				Expiry:    time.Second,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok5)

			// let it expire
			time.Sleep(time.Second + time.Millisecond)

			// it is now invalid
			_, err = Verify(ctx, pool, tok5, nil)
			require.ErrorIs(t, err, ErrInvalid)

			var countBefore int
			err = pool.QueryOne(ctx, &countBefore, `SELECT COUNT(*) FROM tokens_tokens;`)
			require.NoError(t, err)
			require.NotZero(t, countBefore)

			// call the cleanup of expired tokens
			var countAfter int
			err = Cleanup(ctx, pool)
			require.NoError(t, err)
			err = pool.QueryOne(ctx, &countAfter, `SELECT COUNT(*) FROM tokens_tokens;`)
			require.NoError(t, err)
			require.Less(t, countAfter, countBefore)

			// calling again is a no-op
			var countLast int
			err = Cleanup(ctx, pool)
			require.NoError(t, err)
			err = pool.QueryOne(ctx, &countLast, `SELECT COUNT(*) FROM tokens_tokens;`)
			require.NoError(t, err)
			require.Equal(t, countAfter, countLast)
		})
	}
}
