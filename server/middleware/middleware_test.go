package middleware

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"codeberg.org/mna/karbur/ctxvals"
	"github.com/stretchr/testify/require"
)

type statusHandler int

func (h statusHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(int(h))
}

func TestLimitRequestBodyBytes(t *testing.T) {
	h := LimitRequestBodyBytes(2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.Error(t, err)
		require.ErrorContains(t, err, "body too large")
		require.Equal(t, "ab", string(b))
		w.WriteHeader(500)
	}))

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("", "/", strings.NewReader("abcd"))
	h.ServeHTTP(w, r)

	require.EqualValues(t, 500, w.Code)
}

func TestLimitResponseBodyBytes(t *testing.T) {
	h := LimitResponseBodyBytes(2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := w.Write([]byte("abcd"))
		require.Error(t, err)
		require.ErrorIs(t, err, ErrTooManyBytes)
		require.EqualValues(t, 2, n)
	}))

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("", "/", nil)
	h.ServeHTTP(w, r)

	require.EqualValues(t, 500, w.Code)
}

func TestRequestContentType(t *testing.T) {
	acceptedTypes := []string{"application/json", "text/plain"}
	h := RequestContentType(acceptedTypes)(statusHandler(204))

	// use an accepted type
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/", nil)
	r.Header.Add("Content-Type", "application/json")
	h.ServeHTTP(w, r)

	require.EqualValues(t, 204, w.Code)

	// use a non-supported type
	w = httptest.NewRecorder()
	r, _ = http.NewRequest("POST", "/", nil)
	r.Header.Add("Content-Type", "application/xml")
	h.ServeHTTP(w, r)

	require.EqualValues(t, 415, w.Code)
}

func TestResponseContentType(t *testing.T) {
	offeredTypes := []string{"application/json", "text/plain"}
	h := ResponseContentType(offeredTypes)(statusHandler(204))

	// use an offered type
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("", "/", nil)
	r.Header.Add("Accept", "application/json")
	h.ServeHTTP(w, r)

	require.EqualValues(t, 204, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// use a secondary supported type
	w = httptest.NewRecorder()
	r, _ = http.NewRequest("", "/", nil)
	r.Header.Add("Accept", "application/xml")
	r.Header.Add("Accept", "text/plain")
	h.ServeHTTP(w, r)

	require.EqualValues(t, 204, w.Code)
	require.Equal(t, "text/plain", w.Header().Get("Content-Type"))

	// use a non-supported type
	w = httptest.NewRecorder()
	r, _ = http.NewRequest("", "/", nil)
	r.Header.Add("Accept", "application/xml")
	h.ServeHTTP(w, r)

	require.EqualValues(t, 406, w.Code)
}

func TestRequestID(t *testing.T) {
	cases := []struct {
		header string
		force  bool
		preset string
	}{
		{"X", false, ""},
		{"X", true, ""},
		{"X", false, "abc"},
		{"X", true, "abc"},
		{"RandErr", false, ""},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%v", c), func(t *testing.T) {
			h := RequestID(c.header, c.force)(statusHandler(204))

			if c.header == "RandErr" {
				testForceRandErr = true
				defer func() {
					testForceRandErr = false
				}()
			}

			w := httptest.NewRecorder()
			r, _ := http.NewRequest("", "/", nil)
			if c.preset != "" {
				r.Header.Set(c.header, c.preset)
			}
			h.ServeHTTP(w, r)

			require.EqualValues(t, 204, w.Code)
			reqid := r.Header.Get(c.header)
			require.NotEmpty(t, reqid)
			resid := w.Header().Get(c.header)
			require.NotEmpty(t, resid)
			require.Equal(t, reqid, resid)
			if c.header == "RandErr" {
				// should be all digits
				require.Regexp(t, `^\d+$`, resid)
			}
			t.Log(resid)
		})
	}
}

func TestRequestLimit(t *testing.T) {
	h := RequestLimit(&RequestLimitConfig{
		FillInterval: time.Second,
		Capacity:     2,
	})(statusHandler(204))

	codes := []int{204, 204, 503}
	for _, code := range codes {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("", "/", nil)
		h.ServeHTTP(w, r)

		require.EqualValues(t, code, w.Code)
	}
}

func TestTimeoutHandler(t *testing.T) {
	const timeout = 100 * time.Millisecond

	h := TimeoutHandler(timeout)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wait, _ := strconv.Atoi(r.Header.Get("X-Wait"))
		time.Sleep(time.Duration(wait) * time.Millisecond)
		w.WriteHeader(204)
	}))

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("", "/", nil)
	h.ServeHTTP(w, r)

	require.EqualValues(t, 204, w.Code)

	w = httptest.NewRecorder()
	r, _ = http.NewRequest("", "/", nil)
	r.Header.Set("X-Wait", "150")
	h.ServeHTTP(w, r)

	require.EqualValues(t, 503, w.Code)
}

func TestPanicRecovery(t *testing.T) {
	recoverFn := func(w http.ResponseWriter, r *http.Request, v any, stack []byte) {
		require.Equal(t, io.EOF, v)
		w.WriteHeader(500)
	}

	h := PanicRecovery(recoverFn)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fail, _ := strconv.ParseBool(r.Header.Get("X-Fail"))
		if fail {
			panic(io.EOF)
		}
		w.WriteHeader(204)
	}))

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("", "/", nil)
	h.ServeHTTP(w, r)

	require.EqualValues(t, 204, w.Code)

	w = httptest.NewRecorder()
	r, _ = http.NewRequest("", "/", nil)
	r.Header.Set("X-Fail", "true")
	h.ServeHTTP(w, r)

	require.EqualValues(t, 500, w.Code)
}

func TestLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logFn := func(w http.ResponseWriter, r *http.Request, info map[string]any) {
		require.Equal(t, "ok", info["test"])
		logger.Info("logging")
	}
	h := Logging("", logFn)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxvals.SetKeyValue(r.Context(), "test", "ok")
		w.WriteHeader(204)
	}))
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("", "/", nil)
	h.ServeHTTP(w, r)

	require.EqualValues(t, 204, w.Code)

	var count int
	for s := range strings.Lines(buf.String()) {
		count++
		require.Contains(t, s, "level=INFO")
		require.Contains(t, s, "msg=logging")
	}
	require.Equal(t, 1, count)
}

func TestTrustProxyHeaders(t *testing.T) {
	var trusted bool
	hh := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if trusted {
			require.Equal(t, "1.2.3.4", r.RemoteAddr)
			require.Equal(t, "https", r.URL.Scheme)
			require.Equal(t, "example.org", r.Host)
		} else {
			require.Empty(t, r.RemoteAddr)
			require.Empty(t, r.URL.Scheme)
			require.Empty(t, r.Host)
		}
		w.WriteHeader(204)
	})

	m := map[bool]http.Handler{
		true:  TrustProxyHeaders()(hh),
		false: hh,
	}
	for trust, h := range m {
		t.Run(fmt.Sprintf("trusted: %t", trust), func(t *testing.T) {
			trusted = trust
			w := httptest.NewRecorder()
			r, _ := http.NewRequest("", "/", nil)
			r.Header.Set("X-Real-IP", "1.2.3.4")
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("X-Forwarded-Host", "example.org")
			h.ServeHTTP(w, r)
		})
	}
}

func TestAllowMethodOverride(t *testing.T) {
	var wantMethod string
	hh := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, wantMethod, r.Method)
		w.WriteHeader(204)
	})

	m := map[bool]http.Handler{
		true:  AllowMethodOverride()(hh),
		false: hh,
	}
	for allow, h := range m {
		t.Run(fmt.Sprintf("allow: %t", allow), func(t *testing.T) {
			wantMethod = "POST"
			if allow {
				wantMethod = "PUT"
			}
			w := httptest.NewRecorder()
			r, _ := http.NewRequest("POST", "/", nil)
			r.Header.Set("X-HTTP-Method-Override", "PUT")
			h.ServeHTTP(w, r)
		})
	}
}

func TestCanonicalHost(t *testing.T) {
	// using TrustProxyHeaders to be able to set a scheme and host for the request
	// without spinning up a real server.
	h := TrustProxyHeaders()(CanonicalHost("http://example.org", 302)(statusHandler(204)))

	// no redirect
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("", "/", nil)
	r.Header.Set("X-Forwarded-Host", "example.org")
	r.Header.Set("X-Forwarded-Proto", "http")
	h.ServeHTTP(w, r)

	require.EqualValues(t, 204, w.Code)

	// redirect
	w = httptest.NewRecorder()
	r, _ = http.NewRequest("", "/", nil)
	r.Header.Set("X-Forwarded-Host", "not_quite_example.org")
	r.Header.Set("X-Forwarded-Proto", "http")
	h.ServeHTTP(w, r)

	require.EqualValues(t, 302, w.Code)
	require.Equal(t, "http://example.org/", w.Header().Get("Location"))
}

func TestStripPrefix(t *testing.T) {
	h := StripPrefix("/test")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/rest", r.URL.Path)
		w.WriteHeader(204)
	}))

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("", "/test/rest", nil)
	h.ServeHTTP(w, r)

	require.EqualValues(t, 204, w.Code)
}

func TestCORS(t *testing.T) {
	const origin = "https://example.org"
	h := CORS(&CORSConfig{
		AllowedOrigins: []string{origin},
		AllowedMethods: []string{"GET", "POST"},
	})(statusHandler(204))

	t.Run("preflight", func(t *testing.T) {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("OPTIONS", "/", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", "GET")
		h.ServeHTTP(w, r)

		require.Equal(t, origin, w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("actual request", func(t *testing.T) {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", "/", nil)
		r.Header.Set("Origin", origin)
		h.ServeHTTP(w, r)

		require.EqualValues(t, 204, w.Code)
		require.Equal(t, origin, w.Header().Get("Access-Control-Allow-Origin"))
	})
}

func TestCompress(t *testing.T) {
	body := strings.Repeat("hello", 50)
	h := Compress(&CompressConfig{
		ContentTypes: []string{"text/plain"},
		MinSize:      10,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, err := io.WriteString(w, body)
		require.NoError(t, err)
	}))

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	h.ServeHTTP(w, r)

	require.Equal(t, "gzip", w.Header().Get("Content-Encoding"))

	gr, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	defer gr.Close() //nolint
	got, err := io.ReadAll(gr)
	require.NoError(t, err)
	require.Equal(t, body, string(got))
}

func TestRequestTimeouts(t *testing.T) {
	cases := []struct {
		read, write time.Duration
	}{
		{-1, -1},
		{0, 0},
		{50 * time.Millisecond, 50 * time.Millisecond},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("r=%v,w=%v", c.read, c.write), func(t *testing.T) {
			var called bool
			h := RequestTimeouts(c.read, c.write)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(204)
			}))

			w := httptest.NewRecorder()
			r, _ := http.NewRequest("", "/", nil)
			h.ServeHTTP(w, r)

			require.True(t, called)
			require.EqualValues(t, 204, w.Code)
		})
	}
}

func TestFileServerCustomErrors(t *testing.T) {
	// make permission.txt write-only
	err := os.Chmod("testdata/fileserver/permission.txt", 0o200)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chmod("testdata/fileserver/permission.txt", 0o600)
	})

	mw := FileServerCustomErrors(func(code int, w http.ResponseWriter, r *http.Request) (handled bool) {
		switch code {
		case http.StatusNotFound:
			http.Error(w, "custom", 444)
			return true
		case http.StatusForbidden:
			http.Error(w, "custom", http.StatusForbidden)
			return true
		}
		// leave precondition code unhandled
		return false
	})
	srv := httptest.NewServer(mw(http.FileServer(http.Dir("testdata/fileserver"))))
	defer srv.Close()

	// request to a valid file
	res, err := http.Get(srv.URL + "/file.txt")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, "hello\n", string(body))

	// request to a non-existing file
	res, err = http.Get(srv.URL + "/nosuch.txt")
	require.NoError(t, err)
	require.Equal(t, 444, res.StatusCode)
	body, err = io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, "custom\n", string(body))

	// request to a permission-restricted file
	res, err = http.Get(srv.URL + "/permission.txt")
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, res.StatusCode)
	body, err = io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, "custom\n", string(body))

	// request a valid file with wrong etag, triggering a precondition failed error
	req, err := http.NewRequest("GET", srv.URL+"/file.txt", nil)
	require.NoError(t, err)
	req.Header.Set("If-Match", "wrong-etag-value")
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusPreconditionFailed, res.StatusCode)
	body, err = io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Empty(t, string(body))
}
