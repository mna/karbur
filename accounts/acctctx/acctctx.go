// Package acctctx provides type-safe access to account-related values stored
// in the context.
package acctctx

import (
	"context"
	"encoding/json"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/errors"
)

type ctxKey int

const (
	accountKey     = ctxKey(0)
	sessionIDKey   = ctxKey(1)
	sessionDataKey = ctxKey(2)
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

type sessionData struct {
	data  json.RawMessage
	dirty bool
}

// WithSessionData stores the data associated with the current session in the
// context. This should be the raw data as read from storage (unmodified).
func WithSessionData(ctx context.Context, data json.RawMessage) context.Context {
	return context.WithValue(ctx, sessionDataKey, &sessionData{data: data})
}

// SessionData returns the data associated with the current session, and a
// boolean indicating if it is dirty (if any changes were made since the call
// to WithSessionData).
func SessionData(ctx context.Context) (data json.RawMessage, dirty bool) {
	v := ctx.Value(sessionDataKey)
	if ssnData, _ := v.(*sessionData); ssnData != nil {
		return ssnData.data, ssnData.dirty
	}
	return nil, false
}

// ReplaceSessionData replaces the data associated with the current session
// with the JSON-marshaled version of p, and marks the session data as dirty.
// It always assumes that the data changed when this function is called.
func ReplaceSessionData(ctx context.Context, p any) error {
	v := ctx.Value(sessionDataKey)
	ssnData, _ := v.(*sessionData)
	if ssnData == nil {
		return errors.New("no existing session data to replace")
	}

	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	ssnData.data = json.RawMessage(data)
	ssnData.dirty = true
	return nil
}
