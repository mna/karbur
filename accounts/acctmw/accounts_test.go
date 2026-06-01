package acctmw

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/migrate"
	"codeberg.org/mna/karbur/server/params"
	"codeberg.org/mna/karbur/tokens"
	"github.com/stretchr/testify/require"
)

type deferHandler struct {
	h http.Handler
}

func (d *deferHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.h.ServeHTTP(w, r)
}

func setupAccounts(tb testing.TB, pool pgdb.Pool, handlers map[Action]http.Handler) (*Accounts, *httptest.Server) {
	tb.Cleanup(func() {
		err := pool.Close()
		require.NoError(tb, err)
	})

	// apply the migrations
	mig, err := migrate.New(pool, nil)
	require.NoError(tb, err)
	err = tokens.RegisterMigrations(mig)
	require.NoError(tb, err)
	err = accounts.RegisterMigrations(mig)
	require.NoError(tb, err)
	err = mig.Migrate(ctx)
	require.NoError(tb, err)

	toks := &tokens.Tokens{Conn: pool}
	accts := &Accounts{
		Conn:          pool,
		ParamsDecoder: params.New(),
		Tokens:        toks,
	}
	mux := http.NewServeMux()
	mux.Handle("/register", accts.Register(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := handlers[ActionRegister]; h != nil {
			h.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	mux.Handle("/login", accts.Login(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := handlers[ActionLogin]; h != nil {
			h.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	mux.Handle("/load", accts.Login(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := handlers[ActionLoad]; h != nil {
			h.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	srv := httptest.NewServer(mux)
	tb.Cleanup(func() { srv.Close() })

	return accts, srv
}

func createAccount(tb testing.TB, srvURL, email, pwd string) {
	res, err := http.Post(srvURL+"/register", "application/json", strings.NewReader(fmt.Sprintf(`{"email":%q, "password":%q, "password2":%[2]q}`, email, pwd)))
	require.NoError(tb, err)
	require.Equal(tb, 204, res.StatusCode)
}
