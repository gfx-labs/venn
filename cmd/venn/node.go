package main

import (
	"log/slog"
	"net/http"

	"github.com/gfx-labs/venn/lib/config"
	"github.com/gfx-labs/venn/svc/app/node"
	"github.com/gfx-labs/venn/svc/node/atoms/cacher"
	"github.com/gfx-labs/venn/svc/node/atoms/election"
	"github.com/gfx-labs/venn/svc/node/atoms/headstoreProvider"
	"github.com/gfx-labs/venn/svc/node/atoms/stalker"
	"github.com/gfx-labs/venn/svc/node/atoms/subcenter"
	"github.com/gfx-labs/venn/svc/node/atoms/vennstore"
	"github.com/gfx-labs/venn/svc/node/middlewares/headreplacer"
	"github.com/gfx-labs/venn/svc/node/middlewares/promcollect"
	"github.com/gfx-labs/venn/svc/node/quarks/cluster"
	"github.com/gfx-labs/venn/svc/node/stores/headstores/redihead"
	"github.com/gfx-labs/venn/svc/node/stores/vennstores/chainblock"
	"github.com/gfx-labs/venn/svc/node/stores/vennstores/rediblock"
	"github.com/gfx-labs/venn/svc/shared/services/prom"
	"github.com/gfx-labs/venn/svc/shared/services/redi"
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
