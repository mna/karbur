package acctmw

import (
	"fmt"
	"net/http"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/accounts/acctctx"
	"codeberg.org/mna/karbur/errors"
)

// TODO: group creation must prevent creating those
const (
	accessAnyone        = "?"
	accessAuthenticated = "*"
	accessVerified      = "@"
)

// Authorize is a middleware that allows access to h if the request's account
// is a member of one of the groups.
//
// Three special groups exist, "?" for anyone, including
// anonymous/unauthenticated requests, "*" for any authenticated requests, and
// "@" for any authenticated and verified requests.
func (a *Accounts) Authorize(groups []string) func(h http.Handler) http.Handler {
	return a.authorize(groups, false)
}

// Deny is a middleware that denies access to h if the request's account is a
// member of one of the groups.
//
// Three special groups exist, "?" for anyone, including
// anonymous/unauthenticated requests, "*" for any authenticated requests, and
// "@" for any authenticated and verified requests.
func (a *Accounts) Deny(groups []string) func(h http.Handler) http.Handler {
	return a.authorize(groups, true)
}

func (a *Accounts) authorize(groups []string, isDeny bool) func(h http.Handler) http.Handler {
	set := make(map[string]bool, len(groups))
	for _, g := range groups {
		set[g] = true
	}

	return func(h http.Handler) http.Handler {
		isInGroup := func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }
		isNotInGroup := func(w http.ResponseWriter, r *http.Request) {
			err := errors.TagNew("permission denied", accounts.AccountsTag,
				"code", fmt.Sprint(http.StatusForbidden), "action", string(ActionAuthorize))
			a.ErrorHandler(w, r, err)
		}
		if isDeny {
			isInGroup, isNotInGroup = isNotInGroup, isInGroup
		}

		if len(set) == 0 {
			return http.HandlerFunc(isNotInGroup)
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if set[accessAnyone] {
				isInGroup(w, r)
				return
			}

			acct := acctctx.Account(r.Context())
			if acct != nil {
				if set[accessAuthenticated] {
					isInGroup(w, r)
					return
				}
				if acct.Verified.Valid && set[accessVerified] {
					isInGroup(w, r)
					return
				}
				for _, g := range acct.Groups {
					if set[g] {
						isInGroup(w, r)
						return
					}
				}
			}
			isNotInGroup(w, r)
		})
	}
}
