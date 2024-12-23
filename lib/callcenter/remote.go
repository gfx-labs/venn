package callcenter

import "gfx.cafe/open/jrpc"

type Remote interface {
	jrpc.Handler

	Close() error
}
