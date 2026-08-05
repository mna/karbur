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
	accountKey = ctxKey(0)
	sessionKey = ctxKey(1)
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

// WithSession returns a context that holds the specified session information.
// Unlike ResetSession, WithSession always stores a new session entry in the
// context. It should be used for the initial set of the session, typically
// called automatically by the Session middleware.
func WithSession(ctx context.Context, ssnID string, ssnData json.RawMessage) context.Context {
	return context.WithValue(ctx, sessionKey, &session{id: ssnID, data: ssnData})
}

func ResetSession(ctx context.Context, ssnID string, ssnData json.RawMessage) context.Context {
	// TODO: replace an existing sessionKey value with those args, panic if none.
	panic("unimplemented")
}

type session struct {
	id    string
	data  json.RawMessage
	dirty bool
}

// SessionID returns the session ID stored in the context or an empty string if
// there is none.
func SessionID(ctx context.Context) string {
	v := ctx.Value(sessionKey)
	if ssn, _ := v.(*session); ssn != nil {
		return ssn.id
	}
	return ""
}

// Session returns the current session id and its associated data, along with a
// boolean indicating if the data is dirty (if any changes were made since the
// call to WithSession).
func Session(ctx context.Context) (ssnID string, data json.RawMessage, dirty bool) {
	v := ctx.Value(sessionKey)
	if ssn, _ := v.(*session); ssn != nil {
		return ssn.id, ssn.data, ssn.dirty
	}
	return "", nil, false
}

// ReplaceSessionData replaces the data associated with the current session
// with the JSON-marshaled version of p, and marks the session data as dirty.
// It always assumes that the data changed when this function is called.
func ReplaceSessionData(ctx context.Context, p any) error {
	v := ctx.Value(sessionKey)
	ssn, _ := v.(*session)
	if ssn == nil {
		return errors.New("no existing session data to replace")
	}

	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	ssn.data = json.RawMessage(data)
	ssn.dirty = true
	return nil
}
