package callcenter

import (
	"gfx.cafe/open/jrpc"
	"log/slog"
)

// Logger logs each request.
type Logger struct {
	logger *slog.Logger
}

func NewLogger(logger *slog.Logger) *Logger {
	return &Logger{
		logger: logger,
	}
}

func (T *Logger) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		next.ServeRPC(w, r)
		T.logger.Debug("handled request", "method", r.Method)
	})
}
