package callcenter

import (
	"gfx.cafe/open/jrpc"
	"golang.org/x/time/rate"
)

// Ratelimiter limits the amount of requests that can be made to a remote.
type Ratelimiter struct {
	limiter *rate.Limiter
}

func NewRatelimiter(limit rate.Limit, burst int) *Ratelimiter {
	limiter := rate.NewLimiter(limit, burst)

	return &Ratelimiter{
		limiter: limiter,
	}
}

func (T *Ratelimiter) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		if !T.limiter.Allow() {
			_ = w.Send(nil, ErrRatelimited)
			return
		}

		next.ServeRPC(w, r)
	})
}
