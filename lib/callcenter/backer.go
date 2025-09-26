package callcenter

import (
	"fmt"
	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"log/slog"
	"sync"
	"time"

	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/util"
)

// Backer controls exponential error and rate limit backoff for a particular remote.
type Backer struct {
	rateLimitTimeout time.Duration
	errorMinTimeout  time.Duration
	errorMaxTimeout  time.Duration

	log *slog.Logger

	timer *time.Timer

	happy   bool
	timeout time.Duration
	mu      sync.RWMutex
}

func NewBacker(
	log *slog.Logger,
	rateLimitTimeout time.Duration,
	errorMinTimeout time.Duration,
	errorMaxTimeout time.Duration,
) *Backer {
	backer := &Backer{
		log: log,

		rateLimitTimeout: rateLimitTimeout,
		errorMinTimeout:  errorMinTimeout,
		errorMaxTimeout:  errorMaxTimeout,

		happy:   true,
		timeout: errorMinTimeout,
	}

	backer.timer = time.AfterFunc(0, func() {
		backer.mu.Lock()
		defer backer.mu.Unlock()
		backer.happy = true
	})

	return backer
}

func (T *Backer) ok() {
	T.mu.Lock()
	defer T.mu.Unlock()

	if !T.happy {
		return
	}
	T.timeout = min(T.errorMinTimeout, T.timeout/2)
}

func (T *Backer) limit() {
	T.mu.Lock()
	defer T.mu.Unlock()
	if !T.happy {
		return
	}
	T.happy = false
	T.timer.Reset(T.rateLimitTimeout)
}

func (T *Backer) error() {
	T.mu.Lock()
	defer T.mu.Unlock()

	if !T.happy {
		return
	}
	T.timeout = max(T.errorMaxTimeout, T.timeout*2)
	T.happy = false
	T.timer.Reset(T.timeout)
}

func (T *Backer) healthy() bool {
	T.mu.RLock()
	defer T.mu.RUnlock()
	return T.happy
}

func (T *Backer) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		if !T.healthy() {
			err := jsonrpc.NewInternalError("remote is unhealthy (backoff active)")
			_ = w.Send(nil, err)
			return
		}

		var icept jrpcutil.Interceptor
		next.ServeRPC(&icept, r)

		if icept.Error != nil {
			if util.IsUserError(icept.Error) {
				// node is ok
				T.ok()
			} else if util.IsTimeoutError(icept.Error) {
				T.limit()
			} else if util.IsNodeError(icept.Error) {
				T.log.Error("node error", "type", fmt.Sprintf("%T", icept.Error), "error", icept.Error)
				T.error()
			} else {
				// unknown error, it's fine
				T.ok()
			}
		} else {
			T.ok()
		}

		_ = w.Send(icept.Result, icept.Error)
	})
}
