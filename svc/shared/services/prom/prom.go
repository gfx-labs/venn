package prom

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
)

type Prometheus struct {
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
			return nil
		},
	})

	return PrometheusResult{
		Prometheus: p,
	}
}
