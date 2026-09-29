package callcenter

import "github.com/gfx-labs/jrpc"

type Remote interface {
	jrpc.Handler
}

type Middleware interface {
	Middleware(next jrpc.Handler) jrpc.Handler
}
