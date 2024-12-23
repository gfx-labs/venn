package util

import (
	"context"
	"math"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"golang.org/x/sync/semaphore"
)

type Waiter struct {
	sema *semaphore.Weighted
}

func NewWaiter() *Waiter {
	return &Waiter{
		sema: semaphore.NewWeighted(math.MaxInt64),
	}
}

func (T *Waiter) startRequest() {
	// should never block unless after wait, ignoring error is fine
	_ = T.sema.Acquire(context.Background(), 1)
}

func (T *Waiter) endRequest() {
	T.sema.Release(1)
}

func (T *Waiter) Middleware(fn jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		T.startRequest()
		defer T.endRequest()
		fn.ServeRPC(w, r)
	})
}

// Wait will wait until all active requests are finished or ctx is cancelled. Requests after calling Wait will block
// forever
func (T *Waiter) Wait(ctx context.Context) error {
	return T.sema.Acquire(ctx, math.MaxInt64)
}
