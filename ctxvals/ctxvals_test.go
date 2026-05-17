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
