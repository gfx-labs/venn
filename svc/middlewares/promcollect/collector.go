package promcollect

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/subctx"
	"gfx.cafe/gfx/venn/svc/services/prom"
)

type Collector struct {
	logger *slog.Logger
	p      *prom.Prometheus
}

type CollectorParams struct {
	fx.In

	Logger     *slog.Logger
	Prometheus *prom.Prometheus
}

type CollectorResult struct {
	fx.Out

	Collector *Collector
}

func New(params CollectorParams) CollectorResult {
	c := &Collector{
		logger: params.Logger,
		p:      params.Prometheus,
	}

	return CollectorResult{
		Collector: c,
	}
}

func (T *Collector) Middleware(fn jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		if strings.HasSuffix(r.Method, "_subscribe") {
			// no need to log subscriptions
			fn.ServeRPC(w, r)
			return
		}

		start := time.Now()

		var icept jrpcutil.Interceptor

		defer func() {
			dur := time.Since(start)

			chain, err := subctx.GetChain(r.Context())
			if err != nil {
				T.logger.Warn("failed to record metrics", "error", err)
				return
			}

			label := prom.RequestLabel{
				Chain:   chain.Name,
				Method:  r.Method,
				Success: icept.Error == nil,
			}

			prom.Requests.Latency(label).Observe(dur.Seconds() * 1000)
			lvl := slog.LevelDebug
			extra := []any{
				"chain", chain.Name,
				"method", r.Method,
				"transport", r.Peer.Transport,
				"remote_addr", r.Peer.RemoteAddr,
				"params", string(r.Params),
				"duration", dur,
			}
			if icept.Error != nil {
				lvl = slog.LevelError
				extra = append(extra, "err", icept.Error)
			}
			T.logger.Log(context.Background(), lvl, "request",
				extra...,
			)
		}()

		fn.ServeRPC(&icept, r)

		_ = w.Send(icept.Result, icept.Error)
	})
}
