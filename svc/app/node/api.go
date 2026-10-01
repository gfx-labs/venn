package node

import (
	"context"
	"net"
	"net/http"

	"github.com/gfx-labs/venn/svc/node/middlewares/forger"
	"github.com/gfx-labs/venn/svc/shared/services/redi"

	"github.com/gfx-labs/venn/lib/vtrace"
	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidislimiter"
	"go.opentelemetry.io/otel/attribute"

	"github.com/gfx-labs/jrpc"
	"github.com/gfx-labs/jrpc/contrib/codecs"
	"github.com/gfx-labs/jrpc/contrib/extension/subscription"
	"github.com/gfx-labs/jrpc/pkg/jsonrpc"
	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"

	"github.com/gfx-labs/venn/lib/config"
	"github.com/gfx-labs/venn/lib/ratelimit"
	"github.com/gfx-labs/venn/lib/stores/headstore"
	"github.com/gfx-labs/venn/lib/subctx"
	"github.com/gfx-labs/venn/lib/util"
	"github.com/gfx-labs/venn/svc/node/atoms/cacher"
	"github.com/gfx-labs/venn/svc/node/atoms/stalker"
	"github.com/gfx-labs/venn/svc/node/atoms/subcenter"
	"github.com/gfx-labs/venn/svc/node/quarks/cluster"

	"github.com/gfx-labs/venn/dashboard"
	"github.com/gfx-labs/venn/svc/node/middlewares/headreplacer"
	"github.com/gfx-labs/venn/svc/node/middlewares/promcollect"
)

type Params struct {
	fx.In

	Lc         fx.Lifecycle
	Chains     map[string]*config.Chain
	AbuseLimit *config.AbuseLimit

	Redis *redi.Redis

	Subscription *subscription.Engine `optional:"true"`
	// Blockland        *blockland.Blockland   `optional:"true"`
	RequestCollector *promcollect.Collector `optional:"true"`

	// head following for even faster access to the latest block.
	Stalker *stalker.Stalker

	// head replacer middleware for replacing latest block tags
	HeadReplacer *headreplacer.HeadReplacer

	// result caching for certain methods
	Cacher *cacher.Cacher

	// provides direct jsonrpc
	Clusters  *cluster.Clusters
	HeadStore headstore.Store

	// provide subscriptions like eth_subscribe
	Subcenter     *subcenter.Subcenter
	TraceProvider *vtrace.TraceProvider `optional:"true"`
}

type Result struct {
	fx.Out

	Provider jrpc.Handler
	Route    func(r chi.Router) `group:"route"`
}

func New(p Params) (r Result, err error) {

	waiter := util.NewWaiter()
	middlewares := []jrpc.Middleware{
		p.Cacher.Middleware,
		p.HeadReplacer.Middleware,
		(&forger.Forger{Chains: p.Chains}).Middleware,
		p.Subcenter.Middleware,
	}

	if p.RequestCollector != nil {
		middlewares = append(middlewares, p.RequestCollector.Middleware)
	}

	if p.AbuseLimit != nil && p.AbuseLimit.Total > 0 {
		rLimiter, err := rueidislimiter.NewRateLimiter(rueidislimiter.RateLimiterOption{
			ClientBuilder: func(option rueidis.ClientOption) (rueidis.Client, error) {
				return p.Redis.R(), nil
			},
			// TODO: make this configurable
			KeyPrefix: p.Redis.Namespace() + "ratelimit:actions:",
			Limit:     p.AbuseLimit.Total,
			Window:    p.AbuseLimit.Window.Duration,
		})
		// this cant really error because the client builder never errors
		if err != nil {
			return r, err
		}
		middlewares = append(middlewares, ratelimit.RuedisRatelimiter(rLimiter))
	}

	if p.Subscription != nil {
		middlewares = append(middlewares, p.Subscription.Middleware())
	}

	middlewares = append(middlewares, ratelimit.WithIdentifier(func(r *jrpc.Request) (*ratelimit.Identifier, error) {
		slug, _, err := net.SplitHostPort(r.Peer.RemoteAddr)
		if err == nil {
			slug = r.Peer.RemoteAddr
		}
		return &ratelimit.Identifier{
			Endpoint: "venn$internal",
			Type:     "ip",
			Slug:     slug,
		}, nil
	}))

	// waiter is last before otel tracing
	middlewares = append(middlewares, waiter.Middleware)
	// otel tracing
	middlewares = append(middlewares, vtrace.RPCMiddleware(func(req *jsonrpc.Request) []attribute.KeyValue {
		chain, _ := subctx.GetChain(req.Context())
		return []attribute.KeyValue{attribute.String("chain", chain.Name)}
	}))

	rootJrpcHandler := p.Clusters.Middleware(nil)
	handler := jrpc.Handler(rootJrpcHandler)
	for _, m := range middlewares {
		if m != nil {
			handler = m(handler)
		}
	}

	// Add validation middleware last so it executes first
	handler = util.MethodValidationMiddleware()(handler)

	r.Provider = handler

	// add the waiter hook to the shutdown handler.
	p.Lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			if err := waiter.Wait(ctx); err != nil {
				return err
			}
			return nil
		},
	})

	// bind the jrpc handler to a http+websocket codec to host on the http server
	serverHandler := codecs.HttpWebsocketHandler(handler, nil)
	// mount the http server
	r.Route = func(r chi.Router) {
		r.Use(vtrace.ExtractContext)

		for _, chain := range p.Chains {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(subctx.WithChain(r.Context(), chain))
				serverHandler.ServeHTTP(w, r)
			})
			r.Mount("/"+chain.Name, handler)
			for _, alias := range chain.Aliases {
				r.Mount("/"+alias, handler)
			}
		}
		// health check
		r.Mount("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		}))

		// Mount dashboard
		dashboardHandler := dashboard.NewHandler(p.Chains, p.Clusters, p.HeadStore)
		dashboardHandler.Mount(r)
	}
	return
}
