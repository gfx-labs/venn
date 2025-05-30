package callcenter

import (
	"log/slog"

	"gfx.cafe/open/jrpc"
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
		T.logger.Debug("sending remote request", "method", r.Method, "params", r.Params)
	})
}
