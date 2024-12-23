package callcenter

import "gfx.cafe/open/jrpc/pkg/jsonrpc"

var (
	ErrRatelimited         = jsonrpc.NewInternalError("remote is rate limited")
	ErrUnhealthy           = jsonrpc.NewInternalError("remote is unhealthy")
	ErrMethodNotAllowed    = jsonrpc.NewInvalidRequestError("method not allowed")
	ErrHeadJumpedBackwards = jsonrpc.NewInternalError("head jumped backwards")
	ErrHeadOld             = jsonrpc.NewInternalError("head old")
)
