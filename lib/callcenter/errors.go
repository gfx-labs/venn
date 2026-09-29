package callcenter

import "github.com/gfx-labs/jrpc/pkg/jsonrpc"

var (
	ErrRatelimited         = jsonrpc.NewInternalError("remote is rate limited")
	ErrUnhealthy           = jsonrpc.NewInternalError("remote is unhealthy")
	ErrMethodNotAllowed    = jsonrpc.NewInvalidRequestError("method not allowed")
	ErrHeadJumpedBackwards = jsonrpc.NewInternalError("head jumped backwards")
	ErrHeadOld             = jsonrpc.NewInternalError("head old")
)
