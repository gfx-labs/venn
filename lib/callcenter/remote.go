package callcenter

import "gfx.cafe/open/jrpc"

type Remote interface {
	jrpc.Handler
}

type Middleware interface {
	Middleware(next jrpc.Handler) jrpc.Handler
}
