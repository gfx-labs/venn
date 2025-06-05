package util

import (
	"strings"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
)

// MethodValidationMiddleware returns a middleware that validates method names.
// It rejects methods containing "_internal" or longer than 1024 characters.
func MethodValidationMiddleware() jrpc.Middleware {
	return func(next jrpc.Handler) jrpc.Handler {
		return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
			if len(r.Method) > 1024 {
				_ = w.Send(nil, jsonrpc.NewInvalidRequestError("method name too long"))
				return
			}
			if strings.Contains(r.Method, "_internal") {
				_ = w.Send(nil, jsonrpc.NewInvalidRequestError("method not allowed"))
				return
			}
			next.ServeRPC(w, r)
		})
	}
}