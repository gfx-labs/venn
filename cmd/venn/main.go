package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"

	"gfx.cafe/open/gun"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"gfx.cafe/util/go/fxplus"
	"gfx.cafe/util/go/gotel"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/svc/atoms/cacher"
	"gfx.cafe/gfx/venn/svc/atoms/election"
	"gfx.cafe/gfx/venn/svc/atoms/forger"
	"gfx.cafe/gfx/venn/svc/atoms/stalker"
	"gfx.cafe/gfx/venn/svc/atoms/subcenter"
	"gfx.cafe/gfx/venn/svc/atoms/vennstore"
	"gfx.cafe/gfx/venn/svc/handler"
	"gfx.cafe/gfx/venn/svc/middlewares/promcollect"
	"gfx.cafe/gfx/venn/svc/middlewares/ratelimit"
	"gfx.cafe/gfx/venn/svc/quarks/cluster"
	"gfx.cafe/gfx/venn/svc/services/prom"
	"gfx.cafe/gfx/venn/svc/services/redi"
	"gfx.cafe/gfx/venn/svc/stores/headstores/redihead"
	"gfx.cafe/gfx/venn/svc/stores/vennstores/chainblock"
	"gfx.cafe/gfx/venn/svc/stores/vennstores/rediblock"
)

var Config struct {
	SERVERCONFIG_PATH string
}

func init() {
	gun.Load(&Config)
	if Config.SERVERCONFIG_PATH == "" {
		Config.SERVERCONFIG_PATH = "venn.hcl"
	}

	subscription.SetServiceMethodSeparator("_")
}

func main() {

	fx.New(
		fxplus.WithLogger,
		// utility services (universe)
		fx.Provide(
			fxplus.Component("venn"),
			config.FileParser(Config.SERVERCONFIG_PATH),
			NewHttpRouter,
			NewHttpServer,
			fxplus.Context,
		),
		// services (databases, external utilities)
		fx.Provide(
			prom.New,
			redi.New,
		),
		// simple services (quarks)
		fx.Provide(
			// blocktarget.New,
			cluster.New,
		),
		// stores
		fx.Provide(
			chainblock.New,
			rediblock.New,
			redihead.New,
		),
		// more complicated services (atoms)
		fx.Provide(
			subcenter.New,
			election.New,
			vennstore.New,
			cacher.New,
			stalker.New,
			forger.New,
		),
		// middlewares
		fx.Provide(
			NewSubscriptionEngine,
			// blockland.New,
			promcollect.New,
			ratelimit.New,
		),
		// http handler
		fx.Provide(
			handler.New,
		),
		// OTEL tracing
		fx.Provide(
			gotel.NewTraceProvider,
		),
		fx.Invoke(
			fxplus.StatLogger,
			func(*http.Server) {},
			NewHeadLogger,
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
					if err := http.ListenAndServe(bind, nil); err != nil {
						l.Error("failed to start metrics", "err", err)
					}
				}()
				return
			},
		),
	).Run()
}
