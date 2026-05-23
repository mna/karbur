package ctxvals

import (
	"context"
	"log/slog"
	"maps"
	"net"
	"net/http"
)

type ctxKey int

const (
	loggerKey      = ctxKey(0)
	logKeyValueKey = ctxKey(1)
)

// WithLogger returns a context that holds the specified logger.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// Logger returns the logger stored in context or nil if not found in the
// context.
func Logger(ctx context.Context) *slog.Logger {
	v := ctx.Value(loggerKey)
	l, _ := v.(*slog.Logger)
	return l
}

// LoggerOr is like Logger except that it returns the default logger if none is
// found in the context, so that a valid logger is always returned.
func LoggerOr(ctx context.Context) *slog.Logger {
	l := Logger(ctx)
	if l == nil {
		l = slog.Default()
	}
	return l
}

// WithKeyValue returns a context that holds a container for extra logging
// key-value pairs.
func WithKeyValue(ctx context.Context) context.Context {
	return context.WithValue(ctx, logKeyValueKey, make(map[string]any))
}

// SetKeyValue sets the key-value pair in the extra logging container. If
// WithLogKeyValue has not been called previously for the provided context,
// the key-value pair is silently dropped.
func SetKeyValue(ctx context.Context, key string, value any) {
	m, ok := ctx.Value(logKeyValueKey).(map[string]any)
	if ok {
		m[key] = value
	}
}

// ConsumeKeyValuePairs returns the map of key-value pairs added to the context.
// The context map is cleared on return.
func ConsumeKeyValuePairs(ctx context.Context) map[string]any {
	m, _ := ctx.Value(logKeyValueKey).(map[string]any)
	mm := maps.Clone(m)
	clear(m)
	return mm
}

// HTTPServer returns the http server value used to serve the request
// associated with this context, if any. It returns nil if the context is not
// related to an HTTP request.
func HTTPServer(ctx context.Context) *http.Server {
	v := ctx.Value(http.ServerContextKey)
	s, _ := v.(*http.Server)
	return s
}

// LocalAddr returns the local address the connection arrived on for the HTTP
// request associated with this context, if any. It returns nil if the context
// is not related to an HTTP request.
func LocalAddr(ctx context.Context) net.Addr {
	v := ctx.Value(http.LocalAddrContextKey)
	a, _ := v.(net.Addr)
	return a
}
