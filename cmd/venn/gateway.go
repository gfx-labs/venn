package main

import (
	"log/slog"
	"net/http"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/svc/app/gateway"
	"gfx.cafe/gfx/venn/svc/quarks/telemetry"
	"gfx.cafe/gfx/venn/svc/services/gnat"
	"gfx.cafe/gfx/venn/svc/services/prom"
	"gfx.cafe/gfx/venn/svc/services/redi"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"gfx.cafe/util/go/fxplus"
	"gfx.cafe/util/go/gotel"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
)

type StartGateway struct {
	ConfigFile string `short:"c" help:"config file" env:"GATEWAYCONFIG_PATH" default:"./gateway.yml"`
}

func (o *StartGateway) Run() error {
	godotenv.Load()
	subscription.SetServiceMethodSeparator("_")
	fx.New(
		fxplus.WithLogger,
		// utility services (universe)
		fx.Provide(
			fxplus.Component("gateway"),
			config.GatewayFileParser(o.ConfigFile),
			NewHttpRouter,
			NewHttpServer,
			fxplus.Context,
		),
		// services (databases, external utilities)
		fx.Provide(
			prom.New,
			redi.New,
			gnat.New,
		),
		// simple services (quarks)
		fx.Provide(
			telemetry.New,
		),
		// middlewares
		fx.Provide(
			NewSubscriptionEngine,
		),
		// http handler
		fx.Provide(
			gateway.New,
		),
		// OTEL tracing
		fx.Provide(
			gotel.NewTraceProvider,
		),
		fx.Invoke(
			fxplus.StatLogger,
			func(*http.Server) {},
			func(m *config.Metrics, l *slog.Logger) {
				l.Info("launching")
				bind := ":6060"
				if m != nil {
					if m.Disabled {
						l.Warn("metrics disabled")
						return
					}
					if m.Bind != "" {
						bind = m.Bind
					}
				}
				go func() {
					l.Info("starting metrics server", "bind", bind)
					
					// Create a dedicated mux for the metrics server
					mux := http.NewServeMux()
					mux.Handle("/metrics", promhttp.Handler())
					
					// Proxy pprof endpoints from the default mux
					mux.HandleFunc("/debug/", func(w http.ResponseWriter, r *http.Request) {
						http.DefaultServeMux.ServeHTTP(w, r)
					})
					
					if err := http.ListenAndServe(bind, mux); err != nil {
						l.Error("failed to start metrics", "err", err)
					}
				}()
				return
			},
		),
	).Run()
	return nil
}