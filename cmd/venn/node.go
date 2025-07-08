package main

import (
	"log/slog"
	"net/http"
	"time"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/svc/app/node"
	"gfx.cafe/gfx/venn/svc/node/atoms/cacher"
	"gfx.cafe/gfx/venn/svc/node/atoms/election"
	"gfx.cafe/gfx/venn/svc/node/atoms/headstoreProvider"
	"gfx.cafe/gfx/venn/svc/node/atoms/stalker"
	"gfx.cafe/gfx/venn/svc/node/atoms/subcenter"
	"gfx.cafe/gfx/venn/svc/node/atoms/vennstore"
	"gfx.cafe/gfx/venn/svc/node/middlewares/headreplacer"
	"gfx.cafe/gfx/venn/svc/node/middlewares/promcollect"
	"gfx.cafe/gfx/venn/svc/node/quarks/cluster"
	"gfx.cafe/gfx/venn/svc/node/stores/headstores/redihead"
	"gfx.cafe/gfx/venn/svc/node/stores/vennstores/chainblock"
	"gfx.cafe/gfx/venn/svc/node/stores/vennstores/rediblock"
	"gfx.cafe/gfx/venn/svc/shared/services/prom"
	"gfx.cafe/gfx/venn/svc/shared/services/redi"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"gfx.cafe/util/go/fxplus"
	"gfx.cafe/util/go/gotel"
	"github.com/joho/godotenv"
	"go.uber.org/fx"
)

type StartNode struct {
	ConfigFile string `short:"c" help:"config file" env:"SERVERCONFIG_PATH" default:"./venn.yml"`
}

func (o *StartNode) Run() error {
	godotenv.Load()
	subscription.SetServiceMethodSeparator("_")
	fx.New(
		fx.StartTimeout(15*time.Second), // Increase startup timeout
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
			headstoreProvider.New,
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
			headreplacer.New,
			promcollect.New,
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
