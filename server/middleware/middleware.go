// Package middleware provides middleware commonly-required for Web servers.
// All middleware follow the conventional signature of a function accepting
// an http.Handler and returning an http.Handler, making them compatible with
// a wide variety of third-party Go packages that compose middleware (such as
// the popular github.com/justinas/alice package).
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"maps"
	"net/http"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/CAFxX/httpcompression"
	"github.com/felixge/httpsnoop"
	"github.com/gorilla/handlers"
	"github.com/jub0bs/fcors"
	"github.com/juju/ratelimit"
	"github.com/mna/karbur/ctxvals"
	"github.com/mna/karbur/errors"
)

// ErrTooManyBytes is returned by the LimitResponseBodyBytes middleware if too
// many bytes get written to the response body.
const ErrTooManyBytes = errors.ConstError("too many bytes written to the response")

// LimitRequestBodyBytes returns a middleware that limits the number of bytes
// that can be read from the request's body to limit. If more bytes are read,
// it fails the read with an error of type *http.MaxBytesError.
func LimitRequestBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limit > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			h.ServeHTTP(w, r)
		})
	}
}

// LimitResponseBodyBytes returns a middleware that limits the number of bytes
// that can be written to the response. If the number of bytes written exceeds
// the limit, the write fails with ErrTooManyBytes and the HTTP response header
// is written with status code 500 (which may or may not be sent to the client,
// depending if the headers were already written).
func LimitResponseBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limit > 0 {
				w = httpsnoop.Wrap(w, httpsnoop.Hooks{
					Write: func(fn httpsnoop.WriteFunc) httpsnoop.WriteFunc {
						return limitWrite(w, limit, fn)
					},
				})
			}
			h.ServeHTTP(w, r)
		})
	}
}

// limitWrite returns a function with the same signature as io.Writer
// that fails the write when more than limit bytes have been written.
func limitWrite(w http.ResponseWriter, limit int64, fn httpsnoop.WriteFunc) httpsnoop.WriteFunc {
	var size int64

	return func(b []byte) (int, error) {
		var err error

		total := size + int64(len(b))
		if total > limit {
			// this write will result in too many bytes, limit the bytes written
			diff := int(total - limit)
			err = ErrTooManyBytes
			w.WriteHeader(http.StatusInternalServerError)
			b = b[:len(b)-diff]
		}

		n, werr := fn(b)
		size += int64(n)
		if werr != nil {
			err = werr
		}
		return n, err
	}
}

// RequestContentType returns a middleware that validates that the request's
// content-type is one of the supported types of the server. It fails the
// request with status code 415 Unsupported Media Type if that's not the case.
// This uses the github.com/gorilla/handlers third-party package.
//
// The content type is the MIME type format, e.g. "application/json".
func RequestContentType(contentTypes []string) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return handlers.ContentTypeHandler(h, contentTypes...)
	}
}

// ResponseContentType returns a middleware that validates the Accept header
// passed by the request against the offered types by the server. If no
// supported content type is available, it fails the request with status code
// 406 Not Acceptable. Otherwise it sets the response's Content-Type header to
// the negotiated type.
func ResponseContentType(offeredTypes []string) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ct := negotiateContentType(r, offeredTypes, "-")
			if ct == "-" {
				http.Error(w, fmt.Sprintf("No acceptable content type %q; expected one of %q", r.Header.Get("Accept"), offeredTypes), http.StatusNotAcceptable)
				return
			}

			w.Header().Set("Content-Type", ct)
			h.ServeHTTP(w, r)
		})
	}
}

// TimeoutHandler returns a middleware that uses http.TimeoutHandler to fail
// the request with status code 503 Service Unavailable if it takes too long to
// run. After such a timeout, writes by the handler to its ResponseWriter will
// return http.ErrHandlerTimeout.
func TimeoutHandler(timeout time.Duration) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.TimeoutHandler(h, timeout, "")
	}
}

var testForceRandErr = false

// RequestID returns a middleware that generates a unique request ID and sets
// it on the specified request's and response's header. If force is true it
// generates a new request ID even if the request header already has a value.
func RequestID(header string, force bool) func(http.Handler) http.Handler {
	const length = 8

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id := r.Header.Get(header); id != "" && !force {
				w.Header().Set(header, id)
				h.ServeHTTP(w, r)
				return
			}

			// the number of random bytes is length / 2 (since we then hex-encode the bytes)
			b := make([]byte, hex.DecodedLen(length))

			var val string
			if _, err := rand.Read(b); err == nil && !testForceRandErr {
				val = hex.EncodeToString(b)
			} else {
				// fallback on timestamp
				ts := time.Now().UnixNano()
				v := strconv.FormatInt(ts, 10)
				if len(v) > length {
					// take the last n bytes, more randomness
					v = v[len(v)-length:]
				}
				val = v
			}
			r.Header.Set(header, val)
			w.Header().Set(header, val)

			h.ServeHTTP(w, r)
		})
	}
}

// RequestLimitConfig is the configuration for the RequestLimit middleware.
type RequestLimitConfig struct {
	// Rate fills the bucket at the rate of rate tokens per second up to the
	// given maximum capacity. Leave FillInterval <= 0 to use a rate-based
	// bucket.
	Rate float64
	// FillInterval filles the bucket at the rate of Quantum token every
	// FillInterval, up to the given maximum capacity.
	FillInterval time.Duration
	// Capacity is the maximum number of tokens in the bucket.
	Capacity int64
	// Quantum indicates the number of tokens to add at FillInterval. If 0, a
	// value of 1 is used.
	Quantum int64
	// MaxWait is the maximum time to wait for tokens to become available. If 0,
	// the middleware will not allow the request through as soon as the required
	// token is not immediately available.
	MaxWait time.Duration
}

// RequestLimit returns a middleware that limits the number of requests to the
// handler using a token bucket and returns a status code 503 Service
// Unavailable when the number of requests exceed the capacity. It uses the
// github.com/juju/ratelimit package for rate-limiting.
func RequestLimit(config *RequestLimitConfig) func(http.Handler) http.Handler {
	var buck *ratelimit.Bucket

	if config.FillInterval > 0 {
		q := config.Quantum
		if q <= 0 {
			q = 1
		}
		buck = ratelimit.NewBucketWithQuantum(config.FillInterval, config.Capacity, q)
	} else {
		buck = ratelimit.NewBucketWithRate(config.Rate, config.Capacity)
	}

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !buck.WaitMaxDuration(1, config.MaxWait) {
				http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
				return
			}
			h.ServeHTTP(w, r)
		})
	}
}

// PanicRecovery returns a middleware that recovers from a panic, calling
// recoverFn with the response writer, the request, the panic value and the
// call stack to handle the response to the failed request.
func PanicRecovery(recoverFn func(http.ResponseWriter, *http.Request, interface{}, []byte)) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					stack := debug.Stack()
					recoverFn(w, r, err, stack)
				}
			}()
			h.ServeHTTP(w, r)
		})
	}
}

// Logging returns a middleware that collects relevant information about the
// request and calls logFn with that information. If logFn is nil, it uses
// ctxvals.LoggerOr to retrieve the logger and logs the information with all
// key-value pairs. If the reqIDHeader is provided, the request ID is retrieved
// and added to the logging data. It uses the github.com/felixge/httpsnoop
// package to capture request metrics.
func Logging(reqIDHeader string, logFn func(http.ResponseWriter, *http.Request, map[string]any)) func(http.Handler) http.Handler {
	if logFn == nil {
		logFn = func(w http.ResponseWriter, r *http.Request, m map[string]any) {
			logger := ctxvals.LoggerOr(r.Context())
			kvs := make([]any, 0, len(m)*2)
			keys := slices.Collect(maps.Keys(m))
			sort.Strings(keys)
			for _, k := range keys {
				v := m[k]
				kvs = append(kvs, k, v)
			}
			logger.Info("http request", kvs...)
		}
	}

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			metrics := httpsnoop.CaptureMetrics(h, w, r)
			end := time.Now()
			m := map[string]interface{}{
				"start":               start,
				"end":                 end,
				"duration":            end.Sub(start),
				"proto":               r.Proto,
				"host":                r.Host,
				"method":              r.Method,
				"uri":                 r.RequestURI,
				"path":                r.URL.Path,
				"origin":              r.Header.Get("Origin"),
				"body_bytes_received": r.ContentLength,
				"user_agent":          r.UserAgent(),
				"remote_addr":         r.RemoteAddr,
				"body_bytes_sent":     metrics.Written,
				"status":              metrics.Code,
			}
			if reqIDHeader != "" {
				m["request_id"] = w.Header().Get(reqIDHeader)
			}
			logFn(w, r, m)
		})
	}
}

// TrustProxyHeaders returns a middleware that inspects common reverse proxy
// headers and sets the corresponding fields in the HTTP request struct. These
// are X-Forwarded-For and X-Real-IP for the remote (client) IP address,
// X-Forwarded-Proto or X-Forwarded-Scheme for the scheme (http|https),
// X-Forwarded-Host for the host and the RFC7239 Forwarded header, which may
// include both client IPs and schemes.
//
// Make sure that you use this middleware if you actually trust/control the
// proxy in front of your Go web server. This middleware uses the
// github.com/gorilla/handlers package.
func TrustProxyHeaders() func(http.Handler) http.Handler {
	return handlers.ProxyHeaders
}

// AllowMethodOverride returns a middleware that checks for the
// X-HTTP-Method-Override header or the _method form key, and overrides (if
// valid) Request.Method with its value. This middleware uses the
// github.com/gorilla/handlers package.
//
// This is especially useful for HTTP clients that don't support many http
// verbs. It isn't secure to override e.g a GET to a POST, so only POST
// requests are considered.  Likewise, the override method can only be a
// "write" method: PUT, PATCH or DELETE.
//
// Form method takes precedence over header method.
//
// Note that for the proper routing to be done, this middleware should run
// before the router's dispatch to the corresponding handler (that is, this
// middleware should wrap the route multiplexer).
func AllowMethodOverride() func(http.Handler) http.Handler {
	return handlers.HTTPMethodOverrideHandler
}

// CanonicalHost returns a middleware that redirects requests to the canonical
// domain. It accepts a domain and a status code (e.g. 301 or 302) and
// redirects clients to this domain. The existing request path is maintained.
//
// Note: If the provided domain is considered invalid by url.Parse or otherwise
// returns an empty scheme or host, clients are not redirected.
//
// This middleware uses the github.com/gorilla/handlers package.
func CanonicalHost(domain string, code int) func(http.Handler) http.Handler {
	return handlers.CanonicalHost(domain, code)
}

// StripPrefix returns a middleware that serves HTTP requests by removing the
// given prefix from the request URL's Path (and RawPath if set) and invoking
// the handler h.
//
// It uses the standard library's http.StripPrefix.
func StripPrefix(prefix string) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.StripPrefix(prefix, h)
	}
}

// CORSConfig provides configuration options for the CORS middleware.
type CORSConfig struct {
	// AllowCredentials allows both anonymous access and credentialed access
	// (e.g. with cookies) when set to true, according to the specified options.
	// Otherwise, only anonymous access is allowed.
	AllowCredentials bool
	// AllowedOrigins is the list of origins allowed to make CORS requests. If a
	// single origin equal to "*" is set, any origin is accepted. See the
	// third-party package's documentation for more details.
	AllowedOrigins []string
	// AllowedMethods is the list of HTTP methods allowed to make CORS requests.
	// If a single method equal to "*" is set, any method is accepted.
	AllowedMethods []string
	// AllowedRequestHeaders is the list of headers accepted as part of the CORS
	// request. Header names are case-insensitive. If a single header equal to
	// "*" is set, any request header is allowed.
	AllowedRequestHeaders []string
	// ExposedResponseHeaders is the list of headers allowed to be exposed to the
	// client side of a CORS request. If a single header equal to "*" is set, all
	// response headers are exposed.
	ExposedResponseHeaders []string
	// MaxAge is the maximum duration to cache preflight responses by clients
	// (browsers). A negative value instructs browsers to eschew caching of
	// preflight responses altogether, while the default value of 0 causes
	// browsers to cache preflight responses with a default max-age value of 5
	// seconds. Note that the value is translated to a number of seconds, smaller
	// fractions are truncated.
	MaxAge time.Duration
}

// CORS returns a middleware that configures CORS requests according to the
// config options. It uses the third-party github.com/jub0bs/fcors package. It
// panics if the configuration is invalid, the fcors package is strict in what
// it accepts.
//
// Because CORS-preflight requests use OPTIONS as their HTTP method, the
// resources to which you apply a CORS middleware should accept OPTIONS
// requests.
func CORS(config *CORSConfig) func(http.Handler) http.Handler {
	var (
		opts          []fcors.OptionAnon
		optsWithCreds []fcors.Option
		pendingErrs   []error
	)

	if len(config.AllowedOrigins) == 1 && config.AllowedOrigins[0] == "*" {
		opts = append(opts, fcors.FromAnyOrigin())
		pendingErrs = append(pendingErrs, errors.New("cannot allow any origin with credentials"))
	} else if len(config.AllowedOrigins) > 0 {
		opt := fcors.FromOrigins(config.AllowedOrigins[0], config.AllowedOrigins[1:]...)
		opts = append(opts, opt)
		optsWithCreds = append(optsWithCreds, opt)
	}

	if len(config.AllowedMethods) == 1 && config.AllowedMethods[0] == "*" {
		opt := fcors.WithAnyMethod()
		opts = append(opts, opt)
		optsWithCreds = append(optsWithCreds, opt)
	} else if len(config.AllowedMethods) > 0 {
		opt := fcors.WithMethods(config.AllowedMethods[0], config.AllowedMethods[1:]...)
		opts = append(opts, opt)
		optsWithCreds = append(optsWithCreds, opt)
	}

	if len(config.AllowedRequestHeaders) == 1 && config.AllowedRequestHeaders[0] == "*" {
		opt := fcors.WithAnyRequestHeaders()
		opts = append(opts, opt)
		optsWithCreds = append(optsWithCreds, opt)
	} else if len(config.AllowedRequestHeaders) > 0 {
		opt := fcors.WithRequestHeaders(config.AllowedRequestHeaders[0], config.AllowedRequestHeaders[1:]...)
		opts = append(opts, opt)
		optsWithCreds = append(optsWithCreds, opt)
	}

	if len(config.ExposedResponseHeaders) == 1 && config.ExposedResponseHeaders[0] == "*" {
		opts = append(opts, fcors.ExposeAllResponseHeaders())
		pendingErrs = append(pendingErrs, errors.New("cannot expose all response headers with credentials"))
	} else if len(config.ExposedResponseHeaders) > 0 {
		opt := fcors.ExposeResponseHeaders(config.ExposedResponseHeaders[0], config.ExposedResponseHeaders[1:]...)
		opts = append(opts, opt)
		optsWithCreds = append(optsWithCreds, opt)
	}

	if config.MaxAge < 0 {
		opt := fcors.MaxAgeInSeconds(0)
		opts = append(opts, opt)
		optsWithCreds = append(optsWithCreds, opt)
	} else if config.MaxAge > 0 {
		opt := fcors.MaxAgeInSeconds(uint(config.MaxAge / time.Second))
		opts = append(opts, opt)
		optsWithCreds = append(optsWithCreds, opt)
	}

	if config.AllowCredentials {
		if len(pendingErrs) > 0 {
			panic(errors.Join(pendingErrs...))
		}
		if len(optsWithCreds) == 0 {
			panic(errors.New("missing required allowed origin option"))
		}
		mw, err := fcors.AllowAccessWithCredentials(optsWithCreds[0], optsWithCreds[1:]...)
		if err != nil {
			panic(err)
		}
		return mw
	}

	if len(opts) == 0 {
		panic(errors.New("missing required allowed origin option"))
	}
	mw, err := fcors.AllowAccess(opts[0], opts[1:]...)
	if err != nil {
		panic(err)
	}
	return mw
}

// CompressConfig provides configuration options for the compression
// middleware of response bodies.
type CompressConfig struct {
	// ContentTypes lists the response MIME content types to consider using
	// compression. Typically, text content types are more compressable, while
	// many binary content types are already optimized for size.
	ContentTypes []string

	// MinSize specifies the minimum size of the response body to consider using
	// compression. A value <= 0 uses the middleware's default.
	MinSize int
}

// Compress returns a middleware that transparently provides response body
// compression. It uses the github.com/CAFxX/httpcompression third-party
// package.
func Compress(config *CompressConfig) func(http.Handler) http.Handler {
	var opts []httpcompression.Option
	if len(config.ContentTypes) > 0 {
		opts = append(opts, httpcompression.ContentTypes(config.ContentTypes, false))
	}
	if config.MinSize > 0 {
		opts = append(opts, httpcompression.MinSize(config.MinSize))
	}

	mw, err := httpcompression.DefaultAdapter(opts...)
	if err != nil {
		panic(err)
	}
	return mw
}

// TODO: RequestTimeouts to set ResponseController.SetReadDeadline and SetWriteDeadline,
// overriding the http server's timeouts.
