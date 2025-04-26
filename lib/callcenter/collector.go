package callcenter

import (
	"time"

	"gfx.cafe/open/jrpc/pkg/jsonrpc"

	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/svc/services/prom"
)

// Collector collects prometheus stats for this particular remote.
type Collector struct {
	remote     Remote
	chain      string
	name       string
	prometheus *prom.Prometheus
}

func NewCollector(remote Remote, chain, name string, prometheus *prom.Prometheus) *Collector {
	return &Collector{
		remote:     remote,
		chain:      chain,
		name:       name,
		prometheus: prometheus,
	}
}

func (T *Collector) ServeRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
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

	T.remote.ServeRPC(&icept, r)

	_ = w.Send(icept.Result, icept.Error)
}

var _ Remote = (*Collector)(nil)
