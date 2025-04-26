package main

import (
	"gfx.cafe/gfx/venn/svc/app/node"
	"log/slog"
	"net/http"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/svc/atoms/cacher"
	"gfx.cafe/gfx/venn/svc/atoms/election"
	"gfx.cafe/gfx/venn/svc/atoms/stalker"
	"gfx.cafe/gfx/venn/svc/atoms/subcenter"
	"gfx.cafe/gfx/venn/svc/atoms/vennstore"
	"gfx.cafe/gfx/venn/svc/middlewares/promcollect"
	"gfx.cafe/gfx/venn/svc/middlewares/ratelimit"
	"gfx.cafe/gfx/venn/svc/quarks/cluster"
	"gfx.cafe/gfx/venn/svc/services/prom"
	"gfx.cafe/gfx/venn/svc/services/redi"
	"gfx.cafe/gfx/venn/svc/stores/headstores/redihead"
	"gfx.cafe/gfx/venn/svc/stores/vennstores/chainblock"
	"gfx.cafe/gfx/venn/svc/stores/vennstores/rediblock"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"gfx.cafe/util/go/fxplus"
	"gfx.cafe/util/go/gotel"
	"github.com/joho/godotenv"
	"go.uber.org/fx"
)

var cli struct {
	StartNode StartNode `cmd:"start-node" help:"start venn" default:"withargs"`
}

type StartNode struct {
	ConfigFile string `short:"c" help:"config file" env:"SERVERCONFIG_PATH" default:"./venn.yml"`
}

func (o *StartNode) Run() error {
	godotenv.Load()
	subscription.SetServiceMethodSeparator("_")
	fx.New(
		fxplus.WithLogger,
		// utility services (universe)
		fx.Provide(
			fxplus.Component("venn"),
			config.NodeFileParser(o.ConfigFile),
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
			node.New,
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
	return nil
}
