package ctxvals

import (
	"context"
	"log/slog"
	"net"
	"net/http"
)

type ctxKey int

const loggerKey = ctxKey(0)

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
