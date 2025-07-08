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
	chain          string
	name           string
	requestWindow  *rolling.TimePolicy
}

func NewCollector(chain, name string) *Collector {
	return &Collector{
		chain:          chain,
		name:           name,
		requestWindow:  rolling.NewTimePolicy(rolling.NewWindow(600), time.Minute), // 1-minute window, up to 600 points
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

			label := prom.RemoteLabel{
				Chain:   T.chain,
				Remote:  T.name,
				Method:  r.Method,
				Success: icept.Error == nil,
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
