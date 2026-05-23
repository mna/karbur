package ctxvals

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPValues(t *testing.T) {
	t.Run("not in a request", func(t *testing.T) {
		ctx := context.Background()
		s := HTTPServer(ctx)
		require.Nil(t, s)

		a := LocalAddr(ctx)
		require.Nil(t, a)
	})

	t.Run("in a request", func(t *testing.T) {
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := HTTPServer(r.Context())
			require.NotNil(t, s)
			require.Equal(t, srv.Config, s)

			a := LocalAddr(r.Context())
			require.NotNil(t, a)
			require.NotEmpty(t, a.String())

			w.Write([]byte("ok"))
		}))

		res, err := http.Get(srv.URL)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}

func TestLogKeyValue(t *testing.T) {
	ctx := context.Background()

	// no key-value map installed yet
	SetKeyValue(ctx, "a", 1)

	// set the map
	ctx = WithKeyValue(ctx)
	SetKeyValue(ctx, "b", 2)
	SetKeyValue(ctx, "c", 3)
	SetKeyValue(ctx, "b", 4)

	// get the map
	m := ConsumeKeyValuePairs(ctx)
	require.Equal(t, map[string]any{
		"b": 4,
		"c": 3,
	}, m)

	// get it again, it is now empty
	m = ConsumeKeyValuePairs(ctx)
	require.Empty(t, m)

	SetKeyValue(ctx, "d", 5)
	m = ConsumeKeyValuePairs(ctx)
	require.Equal(t, map[string]any{
		"d": 5,
	}, m)
}
