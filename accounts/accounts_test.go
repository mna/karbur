package accounts

import (
	"context"
	"database/sql"
	"testing"

	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/migrate"
	"codeberg.org/mna/karbur/pgdb/pgxadapt"
	"codeberg.org/mna/karbur/pgdb/testdb"
	"codeberg.org/mna/karbur/tokens"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func TestAccounts(t *testing.T) {
	cases := []struct {
		name  string
		setup func() pgdb.Pool
	}{
		{"pgx", func() pgdb.Pool { db := testdb.NewPgx(t, "", ""); return pgxadapt.ToPool(db) }},
		// {"sql", func() pgdb.Pool { db := testdb.NewSQL(t, "", ""); return sqladapt.ToPool(db) }},
		// {"pq", func() pgdb.Pool { db := testdb.NewPqSQL(t, "", ""); return sqladapt.ToPool(db) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := tc.setup()

			mig, err := migrate.New(pool, nil)
			require.NoError(t, err)
			err = tokens.RegisterMigrations(mig)
			require.NoError(t, err)
			err = RegisterMigrations(mig)
			require.NoError(t, err)
			err = mig.Migrate(ctx)
			require.NoError(t, err)

			// load non-existing account
			acct, err := ByEmail(ctx, pool, "nosuch@b")
			require.ErrorIs(t, err, sql.ErrNoRows)
			require.Nil(t, acct)

			acct, err = ByID(ctx, pool, 9999)
			require.ErrorIs(t, err, sql.ErrNoRows)
			require.Nil(t, acct)

			// create a couple accounts
			acct, err = Create(ctx, pool, "a@b", "hashed_pwd")
			require.NoError(t, err)
			require.Equal(t, "a@b", acct.Email)
			require.NotZero(t, acct.ID)

			acct2, err := Create(ctx, pool, "b@c", "hashed_pwd_again")
			require.NoError(t, err)
			require.Equal(t, "b@c", acct2.Email)
			require.NotZero(t, acct2.ID)
			require.NotEqual(t, acct.ID, acct2.ID)

			// load by email and by id works
			got, err := ByEmail(ctx, pool, acct.Email)
			require.NoError(t, err)
			require.Equal(t, acct.ID, got.ID)

			got, err = ByID(ctx, pool, acct2.ID)
			require.NoError(t, err)
			require.Equal(t, acct2.Email, got.Email)

			// unknown still works
			got, err = ByID(ctx, pool, 9999)
			require.ErrorIs(t, err, sql.ErrNoRows)
			require.Nil(t, got)

			// list groups when there are none
			groups, err := Groups(ctx, pool)
			require.NoError(t, err)
			require.Empty(t, groups)

			// create some groups
			err = CreateGroups(ctx, pool, []string{"a", "b"})
			require.NoError(t, err)

			// create with empty array
			err = CreateGroups(ctx, pool, []string{})
			require.NoError(t, err)

			// create with only duplicates
			err = CreateGroups(ctx, pool, []string{"a", "b"})
			require.NoError(t, err)

			// create with duplicates and new ones
			err = CreateGroups(ctx, pool, []string{"a", "c", "d"})
			require.NoError(t, err)

			groups, err = Groups(ctx, pool)
			require.NoError(t, err)
			require.Equal(t, []string{"a", "b", "c", "d"}, groups)

			// set no group for an account
			err = SetGroups(ctx, pool, acct.ID, nil)
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct.ID)
			require.NoError(t, err)
			require.Empty(t, got.Groups)

			// set some groups for the other account, should be unchanged after the tests
			err = SetGroups(ctx, pool, acct2.ID, []string{"a", "d"})
			require.NoError(t, err)

			// set a valid and an unknown group
			err = SetGroups(ctx, pool, acct.ID, []string{"a", "Z"})
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"a"}, got.Groups)

			// add some groups
			err = SetGroups(ctx, pool, acct.ID, []string{"a", "b", "c"})
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"a", "b", "c"}, got.Groups)

			// no-op same list
			err = SetGroups(ctx, pool, acct.ID, []string{"a", "b", "c"})
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"a", "b", "c"}, got.Groups)

			// remove some and add some
			err = SetGroups(ctx, pool, acct.ID, []string{"b", "c", "d"})
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"b", "c", "d"}, got.Groups)

			// remove only, with unkown
			err = SetGroups(ctx, pool, acct.ID, []string{"c", "d", "Z"})
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"c", "d"}, got.Groups)

			// remove all
			err = SetGroups(ctx, pool, acct.ID, []string{})
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct.ID)
			require.NoError(t, err)
			require.Empty(t, got.Groups)

			// check the other account
			got, err = ByID(ctx, pool, acct2.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"a", "d"}, got.Groups)

			// add an already-existing group
			err = AddGroup(ctx, pool, acct2.ID, "a")
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct2.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"a", "d"}, got.Groups)

			// add a new group
			err = AddGroup(ctx, pool, acct2.ID, "b")
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct2.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"a", "b", "d"}, got.Groups)

			// add a non-existing group
			err = AddGroup(ctx, pool, acct2.ID, "Z")
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct2.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"a", "b", "d"}, got.Groups)

			// remove a non-existing group
			err = RemoveGroup(ctx, pool, acct2.ID, "Z")
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct2.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"a", "b", "d"}, got.Groups)

			// remove a group already not applied
			err = RemoveGroup(ctx, pool, acct2.ID, "c")
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct2.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"a", "b", "d"}, got.Groups)

			// remove a group that needs to be removed
			err = RemoveGroup(ctx, pool, acct2.ID, "b")
			require.NoError(t, err)
			got, err = ByID(ctx, pool, acct2.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"a", "d"}, got.Groups)
		})
	}
}
