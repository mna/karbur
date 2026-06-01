// Package acctctx provides type-safe access to account-related values stored
// in the context.
package acctctx

import (
	"context"

	"codeberg.org/mna/karbur/accounts"
)

type ctxKey int

const (
	accountKey   = ctxKey(0)
	sessionIDKey = ctxKey(1)
)

// WithAccount returns a context that holds the specified account. Typically
// this is the currently authenticated account.
func WithAccount(ctx context.Context, acct *accounts.Account) context.Context {
	return context.WithValue(ctx, accountKey, acct)
}

// Account returns the account stored in context or nil if there is none.
func Account(ctx context.Context) *accounts.Account {
	v := ctx.Value(accountKey)
	acct, _ := v.(*accounts.Account)
	return acct
}

// WithSessionID returns a context that holds the specified session ID.
// Typically this is the session ID used to authenticate the current account.
func WithSessionID(ctx context.Context, ssnID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, ssnID)
}

// SessionID returns the session ID stored in the context or an empty string if
// there is none.
func SessionID(ctx context.Context) string {
	v := ctx.Value(sessionIDKey)
	ssnID, _ := v.(string)
	return ssnID
}
