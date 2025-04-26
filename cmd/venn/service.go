package main

import (
	"context"
	"errors"
	"gfx.cafe/gfx/venn/lib/subctx"
	"log/slog"
	"net"
	"net/http"
	"time"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"
)

type HttpRouterParams struct {
	fx.In

	Routes []func(r chi.Router) `group:"route"`
}

type HttpRouterResult struct {
	fx.Out

	Mux *chi.Mux
}

func NewHttpRouter(params HttpRouterParams) HttpRouterResult {
	mux := chi.NewRouter()
	for _, route := range params.Routes {
		mux.Group(route)
	}
	return HttpRouterResult{
		Mux: mux,
	}
}

type HttpServerParams struct {
	fx.In

	Lc     fx.Lifecycle
	Log    *slog.Logger
	Config *config.HTTP
	Mux    *chi.Mux
}

type HttpServerResult struct {
	fx.Out

	Server *http.Server
}

func NewHttpServer(params HttpServerParams) HttpServerResult {
	server := &http.Server{
		Addr:    params.Config.Bind,
		Handler: params.Mux,
	}
	params.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			conf := &net.ListenConfig{Control: reusePort}
			l, err := conf.Listen(context.Background(), "tcp", server.Addr)
			if err != nil {
				return err
			}
			params.Log.Info("starting http server", "addr", server.Addr)
			go func() {
				if err = server.Serve(l); err != nil {
					if !errors.Is(err, http.ErrServerClosed) {
						params.Log.Error("error serving http", "err", err)
					}
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return server.Shutdown(ctx)
		},
	})
	return HttpServerResult{
		Server: server,
	}
}

type SubscriptionEngineResult struct {
	fx.Out

	Engine *subscription.Engine
}

func NewSubscriptionEngine() SubscriptionEngineResult {
	engine := subscription.NewEngine()
	return SubscriptionEngineResult{
		Engine: engine,
	}
}

type HeadLoggerParams struct {
	fx.In

	Ctx      context.Context
	Log      *slog.Logger
	Lc       fx.Lifecycle
	Provider jrpc.Handler
	Chains   map[string]*config.Chain
}

func NewHeadLogger(params HeadLoggerParams) {
	params.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go func() {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-params.Ctx.Done():
						return
					case <-ticker.C:
						logHeads(params.Ctx, params.Log, params.Chains, params.Provider)
					}
				}
			}()
			return nil
		},
	})
}

func logHeads(ctx context.Context, log *slog.Logger, chains map[string]*config.Chain, handler jrpc.Handler) {
	for _, chain := range chains {
		var number hexutil.Uint64
		cctx := subctx.WithChain(ctx, chain)
		if err := jrpcutil.Do(cctx, handler, &number, "eth_blockNumber", nil); err != nil {
			log.Error("logging head", "err", err)
			continue
		}
		log.Info("Head Block", "chain", chain.Name, "block", int(number))
	}
}
