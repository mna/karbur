// Package acctctx provides type-safe access to account-related values stored
// in the context.
package acctctx

import (
	"context"

	"codeberg.org/mna/karbur/accounts"
)

type ctxKey int

const accountKey = ctxKey(0)

// WithAccount returns a context that holds the specified account.
func WithAccount(ctx context.Context, acct *accounts.Account) context.Context {
	return context.WithValue(ctx, accountKey, acct)
}

// Account returns the account stored in context or nil if there are none.
func Account(ctx context.Context) *accounts.Account {
	v := ctx.Value(accountKey)
	acct, _ := v.(*accounts.Account)
	return acct
}
