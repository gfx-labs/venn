package callcenter

import (
	"log/slog"

	"gfx.cafe/open/jrpc/pkg/jsonrpc"
)

// Logger logs each request.
type Logger struct {
	remote Remote
	logger *slog.Logger
}

func NewLogger(remote Remote, logger *slog.Logger) *Logger {
	return &Logger{
		remote: remote,
		logger: logger,
	}
}

func (T *Logger) ServeRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
	T.remote.ServeRPC(w, r)

	T.logger.Debug("handled request", "method", r.Method)
}

var _ Remote = (*Logger)(nil)
