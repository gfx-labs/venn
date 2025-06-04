package callcenter

import (
	"time"

	"gfx.cafe/open/jrpc"

	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/svc/services/prom"
)

// Collector collects prometheus stats for this particular remote.
type Collector struct {
	chain      string
	name       string
	prometheus *prom.Prometheus
}

func NewCollector(chain, name string, prometheus *prom.Prometheus) *Collector {
	return &Collector{
		chain:      chain,
		name:       name,
		prometheus: prometheus,
	}
}

func (T *Collector) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		start := time.Now()

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
