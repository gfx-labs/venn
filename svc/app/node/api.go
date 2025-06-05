package node

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"gfx.cafe/gfx/venn/svc/middlewares/forger"
	"gfx.cafe/gfx/venn/svc/services/redi"

	"gfx.cafe/open/jrpc/contrib/jrpcutil"
	"gfx.cafe/util/go/gotel"
	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidislimiter"
	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/contrib/codecs"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/ratelimit"
	"gfx.cafe/gfx/venn/lib/subctx"
	"gfx.cafe/gfx/venn/lib/util"
	"gfx.cafe/gfx/venn/svc/atoms/cacher"
	"gfx.cafe/gfx/venn/svc/atoms/stalker"
	"gfx.cafe/gfx/venn/svc/atoms/subcenter"
	"gfx.cafe/gfx/venn/svc/quarks/cluster"

	"gfx.cafe/gfx/venn/svc/middlewares/headreplacer"
	"gfx.cafe/gfx/venn/svc/middlewares/promcollect"
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
	Clusters *cluster.Clusters

	// provide subscriptions like eth_subscribe
	Subcenter     *subcenter.Subcenter
	TraceProvider *gotel.TraceProvider `optional:"true"`
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
		(&forger.Forger{}).Middleware,
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
	middlewares = append(middlewares, func(next jrpc.Handler) jrpc.Handler {
		tracer := otel.Tracer("jrpc")

		fn := jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, req *jsonrpc.Request) {
			chain, _ := subctx.GetChain(req.Context())

			ctx, span := tracer.Start(req.Context(), req.Method,
				trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(
					attribute.String("method", req.Method),
					attribute.String("params", string(req.Params)),
					attribute.String("chain", chain.Name)))
			defer span.End()

			ew := &jrpcutil.ErrorRecorder{
				ResponseWriter: w,
			}

			// execute next http handler
			next.ServeRPC(w, req.WithContext(ctx))

			if err := ew.Error(); err != nil {
				span.SetStatus(codes.Error, fmt.Sprintf("error: %s", err))
				span.RecordError(err)
			}
		})
		return fn
	})

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
		r.Use(otelchi.Middleware("venn", otelchi.WithChiRoutes(r), otelchi.WithFilter(
			func(r *http.Request) bool {
				return r.Header.Get("upgrade") == ""
			})))

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
		// TODO: eventually stats/dashboard will be here.
	}
	return
}
