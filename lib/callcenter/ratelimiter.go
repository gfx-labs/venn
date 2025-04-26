package callcenter

import (
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"golang.org/x/time/rate"
)

// Ratelimiter limits the amount of requests that can be made to a remote.
type Ratelimiter struct {
	limiter *rate.Limiter

	remote Remote
}

func NewRatelimiter(remote Remote, limit rate.Limit, burst int) *Ratelimiter {
	limiter := rate.NewLimiter(limit, burst)

	return &Ratelimiter{
		limiter: limiter,
		remote:  remote,
	}
}

func (T *Ratelimiter) ServeRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
	if !T.limiter.Allow() {
		_ = w.Send(nil, ErrRatelimited)
		return
	}

	T.remote.ServeRPC(w, r)
}

var _ Remote = (*Ratelimiter)(nil)
