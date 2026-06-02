package acctmw

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codeberg.org/mna/karbur/accounts"
	"codeberg.org/mna/karbur/pgdb"
	"codeberg.org/mna/karbur/pgdb/migrate"
	"codeberg.org/mna/karbur/server/params"
	"codeberg.org/mna/karbur/tokens"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/publicsuffix"
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
	mux.Handle("/load", accts.Load(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func newBrowserClient(tb testing.TB) *http.Client {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	require.NoError(tb, err)
	return &http.Client{Jar: jar, Timeout: 5 * time.Second}
}

func createAccountWithClient(tb testing.TB, client *http.Client, srvURL, email, pwd string) {
	res, err := client.Post(srvURL+"/register", "application/json",
		strings.NewReader(fmt.Sprintf(`{"email":%q, "password":%q, "password2":%[2]q}`, email, pwd)))
	require.NoError(tb, err)
	require.Equal(tb, 204, res.StatusCode)
}

func createAccount(tb testing.TB, srvURL, email, pwd string) {
	createAccountWithClient(tb, http.DefaultClient, srvURL, email, pwd)
}

func doLoginWithClient(tb testing.TB, client *http.Client, srvURL, email, pwd string) {
	res, err := client.Post(srvURL+"/login", "application/json", strings.NewReader(fmt.Sprintf(`{"email":%q, "password":%q}`, email, pwd)))
	require.NoError(tb, err)
	require.Equal(tb, 204, res.StatusCode)
}

func doLogin(tb testing.TB, srvURL, email, pwd string) {
	doLoginWithClient(tb, http.DefaultClient, srvURL, email, pwd)
}
