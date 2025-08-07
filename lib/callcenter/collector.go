package callcenter

import (
	"time"

	"gfx.cafe/open/jrpc"

	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/svc/shared/services/prom"
	"github.com/asecurityteam/rolling"
)

// Collector collects prometheus stats for this particular remote.
type Collector struct {
	chain         string
	name          string
	requestWindow *rolling.TimePolicy
	successWindow *rolling.TimePolicy
}

func NewCollector(chain, name string) *Collector {
	return &Collector{
		chain:         chain,
		name:          name,
		requestWindow: rolling.NewTimePolicy(rolling.NewWindow(600), 100*time.Millisecond), // 1-minute window: 600 buckets × 100ms each
		successWindow: rolling.NewTimePolicy(rolling.NewWindow(600), 100*time.Millisecond), // 1-minute window: 600 buckets × 100ms each
	}
}

func (T *Collector) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		start := time.Now()

		// Track request
		T.requestWindow.Append(1.0)

		var icept jrpcutil.Interceptor

		defer func() {
			dur := time.Since(start)

			success := icept.Error == nil

			// Track success/failure
			if success {
				T.successWindow.Append(1.0)
			} else {
				T.successWindow.Append(0.0)
			}

			label := prom.RemoteLabel{
				Chain:   T.chain,
				Remote:  T.name,
				Method:  r.Method,
				Success: success,
			}
			prom.Remotes.Latency(label).Observe(dur.Seconds() * 1000)
		}()

		next.ServeRPC(&icept, r)

		_ = w.Send(icept.Result, icept.Error)
	})
}

// GetRequestsPerMinute returns the current requests per minute rate
func (T *Collector) GetRequestsPerMinute() float64 {
	return T.requestWindow.Reduce(func(w rolling.Window) float64 {
		return rolling.Sum(w) // Sum of all request counts in the window
	})
}

// GetSuccessRate returns the success rate as a percentage (0-100)
func (T *Collector) GetSuccessRate() float64 {
	totalRequests := T.requestWindow.Reduce(func(w rolling.Window) float64 {
		return rolling.Count(w)
	})

	if totalRequests == 0 {
		return 100.0 // No requests means 100% success rate
	}

	totalSuccesses := T.successWindow.Reduce(func(w rolling.Window) float64 {
		return rolling.Sum(w)
	})

	return (totalSuccesses / totalRequests) * 100.0
}

// GetChainName returns the chain name for this collector
func (T *Collector) GetChainName() string {
	return T.chain
}
