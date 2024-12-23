package prom

import (
	"context"
	"log/slog"
	"net/http"

	"gfx.cafe/open/gotoprom"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
)

type Prometheus struct {
	Requests Requests `namespace:"requests"`
	Remotes  Remotes  `namespace:"remotes"`
}

type PrometheusParams struct {
	fx.In

	Lc     fx.Lifecycle
	Logger *slog.Logger
}

type PrometheusResult struct {
	fx.Out

	Prometheus *Prometheus
}

func New(params PrometheusParams) PrometheusResult {
	p := &Prometheus{}

	params.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			http.Handle("/metrics", promhttp.Handler())
			return gotoprom.Init(p, "venn", nil)
		},
	})

	return PrometheusResult{
		Prometheus: p,
	}
}
