package tokens

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/migrate"
	"codeberg.org/mna/karbur/pgdb/pgxadapt"
	"codeberg.org/mna/karbur/pgdb/sqladapt"
	"codeberg.org/mna/karbur/pgdb/testdb"
	"github.com/google/uuid"
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

			tt := Tokens{Conn: pool, RawTokenSize: 0}

			// create a token without a type
			_, _, err = tt.New(ctx, TokenArgs{
				Type:           "",
				RefID:          uuid.New(),
				SingleUse:      true,
				AbsoluteExpiry: time.Second,
			})
			require.Error(t, err)
			require.ErrorContains(t, err, "SQLSTATE 23514") // violates check constraint

			// create a single-use token
			tok1RefID := uuid.New()
			tok1, _, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          tok1RefID,
				SingleUse:      true,
				AbsoluteExpiry: time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok1)

			// verify the token
			dbtok, err := tt.Verify(ctx, tok1, MustMatchTypeAndRefID("test", tok1RefID))
			require.NoError(t, err)
			require.Equal(t, tok1, dbtok.Token)
			require.WithinDuration(t, time.Now().Add(time.Minute), dbtok.Expiry, 2*time.Second)

			// verify the token again, now invalid
			_, err = tt.Verify(ctx, tok1, MustMatchTypeAndRefID("test", tok1RefID))
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalid)

			// create another single-use token
			tok2RefID := uuid.New()
			tok2a, _, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          tok2RefID,
				SingleUse:      true,
				AbsoluteExpiry: time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok2a)

			// generate another for the same type/ref, will replace it
			tok2b, _, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          tok2RefID,
				SingleUse:      true,
				AbsoluteExpiry: time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok2b)
			require.NotEqual(t, tok2a, tok2b)

			// verify the initial token, invalid
			_, err = tt.Verify(ctx, tok2a, nil)
			require.ErrorIs(t, err, ErrInvalid)

			// verify the new token, valid
			_, err = tt.Verify(ctx, tok2b, nil)
			require.NoError(t, err)

			// generate a multi-use token without data
			tok3RefID := uuid.New()
			tok3, data, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          tok3RefID,
				SingleUse:      false,
				AbsoluteExpiry: time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok3)
			require.Equal(t, json.RawMessage(`null`), data)

			// verify it, valid
			dbtok, err = tt.Verify(ctx, tok3, MustMatchType("test"))
			require.NoError(t, err)
			require.Equal(t, tok3, dbtok.Token)
			require.EqualValues(t, tok3RefID, dbtok.RefID)

			// verify it with a non-matching type, invalid
			_, err = tt.Verify(ctx, tok3, MustMatchType("NO-SUCH-TYPE"))
			require.ErrorIs(t, err, ErrInvalid)

			// verify it again, still valid
			dbtok, err = tt.Verify(ctx, tok3, MustMatchType("test"))
			require.NoError(t, err)
			require.EqualValues(t, tok3RefID, dbtok.RefID)

			// can create another multi-use for the same type/ref
			tok4, _, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          tok3RefID,
				SingleUse:      false,
				AbsoluteExpiry: time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok4)
			require.NotEqual(t, tok3, tok4)

			// both are still valid
			dbtok, err = tt.Verify(ctx, tok3, MustMatchType("test"))
			require.NoError(t, err)
			require.EqualValues(t, tok3RefID, dbtok.RefID)
			dbtok, err = tt.Verify(ctx, tok4, MustMatchType("test"))
			require.NoError(t, err)
			require.EqualValues(t, tok3RefID, dbtok.RefID)

			// create a short-lived multi-use with non-nil data
			tok5, data, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          uuid.New(),
				SingleUse:      false,
				AbsoluteExpiry: time.Second,
				Data:           map[string]int{"x": 1},
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok5)
			require.Equal(t, json.RawMessage(`{"x":1}`), data)

			// update its data
			err = tt.UpdateData(ctx, tok5, json.RawMessage(`{"x":2}`))
			require.NoError(t, err)

			// let it expire
			time.Sleep(time.Second + time.Millisecond)

			// it is now invalid
			_, err = tt.Verify(ctx, tok5, nil)
			require.ErrorIs(t, err, ErrInvalid)

			var countBefore int
			err = pool.QueryOne(ctx, &countBefore, `SELECT COUNT(*) FROM tokens_tokens;`)
			require.NoError(t, err)
			require.NotZero(t, countBefore)

			// call the cleanup of expired tokens
			var countAfter int
			err = tt.Cleanup(ctx)
			require.NoError(t, err)
			err = pool.QueryOne(ctx, &countAfter, `SELECT COUNT(*) FROM tokens_tokens;`)
			require.NoError(t, err)
			require.Less(t, countAfter, countBefore)

			// calling again is a no-op
			var countLast int
			err = tt.Cleanup(ctx)
			require.NoError(t, err)
			err = pool.QueryOne(ctx, &countLast, `SELECT COUNT(*) FROM tokens_tokens;`)
			require.NoError(t, err)
			require.Equal(t, countAfter, countLast)

			// create a single-use token with an idle expiration, it is ignored
			tok6, _, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          uuid.New(),
				SingleUse:      true,
				AbsoluteExpiry: time.Minute,
				IdleExpiry:     time.Second,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok6)

			// let the idle timeout expire, but does nothing as the idle is not
			// applied
			time.Sleep(time.Second + time.Millisecond)

			// it is still valid
			_, err = tt.Verify(ctx, tok6, nil)
			require.NoError(t, err)

			// create another single-use token with an ignored idle expiration
			tok7, _, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          uuid.New(),
				SingleUse:      true,
				AbsoluteExpiry: time.Minute,
				IdleExpiry:     time.Second,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok7)

			// try to force it to have an idle expiry in the DB, triggers the
			// constraint failure
			_, err = pool.Exec(ctx, `UPDATE "tokens_tokens" SET "idle" = now() WHERE "token" = $1`, tok7)
			require.Error(t, err)
			perr := pgdb.AsProtocolError(err)
			require.Equal(t, "23514", perr.Code)
			require.Equal(t, "chk_idle_multi_use_only", perr.ConstraintName)

			// create a multi-use token with an idle expiry
			tok8, _, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          uuid.New(),
				SingleUse:      false,
				AbsoluteExpiry: time.Minute,
				IdleExpiry:     time.Second,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok8)

			// verify it immediately, it is valid
			_, err = tt.Verify(ctx, tok8, nil)
			require.NoError(t, err)

			time.Sleep(100 * time.Millisecond)

			// verify it within idle timeout, it is valid
			tokv, err := tt.Verify(ctx, tok8, nil)
			require.NoError(t, err)
			require.True(t, tokv.Idle.Valid)
			t.Log(tokv.Idle)

			// let the idle expire
			time.Sleep(time.Second + time.Millisecond)

			// now invalid
			_, err = tt.Verify(ctx, tok8, nil)
			require.ErrorIs(t, err, ErrInvalid)

			// create a multi-use token without data
			tok9, data, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          uuid.New(),
				AbsoluteExpiry: time.Minute,
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok9)
			require.Equal(t, json.RawMessage(`null`), data)

			// update its data
			err = tt.UpdateData(ctx, tok9, json.RawMessage(`1`))
			require.NoError(t, err)

			// verify it, it is valid and returns the updated data
			tokv, err = tt.Verify(ctx, tok9, nil)
			require.NoError(t, err)
			require.Equal(t, tok9, tokv.Token)
			require.Equal(t, json.RawMessage(`1`), tokv.Data)
			require.False(t, tokv.Idle.Valid)

			// delete it, it isn't valid anymore
			err = tt.Delete(ctx, tok9)
			require.NoError(t, err)
			_, err = tt.Verify(ctx, tok9, nil)
			require.ErrorIs(t, err, ErrInvalid)

			// create a new multi-use token with data and idle expiry
			tok10RefID := uuid.New()
			tok10, data, err := tt.New(ctx, TokenArgs{
				Type:           "test",
				RefID:          tok10RefID,
				AbsoluteExpiry: time.Minute,
				IdleExpiry:     5 * time.Second,
				Data:           json.RawMessage(`1`),
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok10)
			require.Equal(t, json.RawMessage(`1`), data)

			// verify it, it is valid
			tokv, err = tt.Verify(ctx, tok10, nil)
			require.NoError(t, err)
			require.Equal(t, tok10, tokv.Token)
			require.Equal(t, json.RawMessage(`1`), tokv.Data)
			require.True(t, tokv.Idle.Valid)

			// rotate the token to a new one without idle expiry
			tok10bRefID := uuid.New()
			tok10b, err := tt.Rotate(ctx, tok10, TokenArgs{
				Type:           "zzzz", // ignored
				RefID:          tok10bRefID,
				AbsoluteExpiry: time.Second,
				IdleExpiry:     0,
				Data:           json.RawMessage(`2`), // ignored
			})
			require.NoError(t, err)
			require.NotEmpty(t, tok10b)
			require.NotEqual(t, tok10, tok10b)

			// the old token is not valid anymore
			_, err = tt.Verify(ctx, tok10, nil)
			require.ErrorIs(t, err, ErrInvalid)

			// verify it, the new token is valid
			tokv, err = tt.Verify(ctx, tok10b, nil)
			require.NoError(t, err)
			require.Equal(t, tok10b, tokv.Token)
			require.Equal(t, tok10bRefID, tokv.RefID)         // ref id was updated
			require.Equal(t, json.RawMessage(`1`), tokv.Data) // data was maintained
			require.False(t, tokv.Idle.Valid)                 // idle expiry was removed

			// let it expire
			time.Sleep(time.Second + time.Millisecond)

			// not valid anymore
			_, err = tt.Verify(ctx, tok10b, nil)
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
}
