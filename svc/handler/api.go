package handler

import (
	"context"
	"fmt"
	"net/http"

	"gfx.cafe/open/jrpc/contrib/jrpcutil"
	"gfx.cafe/util/go/gotel"
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
	"gfx.cafe/gfx/venn/lib/subctx"
	"gfx.cafe/gfx/venn/lib/util"
	"gfx.cafe/gfx/venn/svc/atoms/cacher"
	"gfx.cafe/gfx/venn/svc/atoms/forger"
	"gfx.cafe/gfx/venn/svc/atoms/stalker"
	"gfx.cafe/gfx/venn/svc/atoms/subcenter"
	"gfx.cafe/gfx/venn/svc/quarks/cluster"

	"gfx.cafe/gfx/venn/svc/middlewares/promcollect"
	"gfx.cafe/gfx/venn/svc/middlewares/ratelimit"
)

type Params struct {
	fx.In

	Lc           fx.Lifecycle
	Chains       map[string]*config.Chain
	RateLimiter  *ratelimit.Limiter   `optional:"true"`
	Subscription *subscription.Engine `optional:"true"`
	// Blockland        *blockland.Blockland   `optional:"true"`
	RequestCollector *promcollect.Collector `optional:"true"`

	// forge methods that may not exist on all downstream chains
	Forgers forger.Forgers

	// head following for even faster access to the latest block.
	Stalkers stalker.Stalkers

	// result caching for certain methods
	Cachers cacher.Cachers

	// provides direct jsonrpc
	Clusters cluster.Clusters

	// provide subscriptions like eth_subscribe
	Subcenters    subcenter.Subcenters
	TraceProvider *gotel.TraceProvider `optional:"true"`
}

type Result struct {
	fx.Out

	Providers util.Multichain[jrpc.Handler]
	Route     func(r chi.Router) `group:"route"`
}

func New(p Params) (r Result, err error) {
	r.Providers, err = util.MakeMultichain(
		p.Chains,
		func(chain *config.Chain) (jrpc.Handler, error) {
			// dont be intimidated by ChooseChain4.
			// all it does is extract the first match for the chain name from each of the maps.
			// the reason is because a forger has a stalker, a stalker has a cacher, and a cacher has a cluster.
			// a chain needs all the downstream dependencies in order to server the upstream dependencies.
			// in a way, these are "more complicated" middleware.

			// TODO: we should migrate these to more proper middleware.
			return util.ChooseChain4(
				chain.Name,
				p.Forgers,
				p.Stalkers,
				p.Cachers,
				p.Clusters,
			)
		},
	)
	if err != nil {
		return
	}

	// note that the order of middleware being added is opposite to the order in which they are invoked.
	// also, all middleware, if the chain matters, must extract the chain from the request context. including the base level middleware handler, as you see here.
	var handler jrpc.Handler = jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, req *jsonrpc.Request) {
		// extract the chain injected into the request context
		chain, err := subctx.GetChain(req.Context())
		if err != nil {
			_ = w.Send(nil, err)
			return
		}
		// from here, we grab the correct chain from the providers (this is a simple map lookup)
		provider, err := util.GetChain(chain, r.Providers)
		if err != nil {
			_ = w.Send(nil, err)
			return
		}

		// now grab the subcenter for the chain
		subcenter, err := util.GetChain(chain, p.Subcenters)
		// ignore the error, because its if the subcenter doesn't exist, and we just dont mount in that case.
		if err == nil {
			provider = subcenter.Middleware(provider)
		}

		// and then we process the actual request using the chains handler
		// in this way, each chain is isolated from the others, but of course are all in the same app
		provider.ServeRPC(w, req)
	})

	// blockland should be mounted last
	/* if p.Blockland != nil {
		handler = p.Blockland.Middleware(handler)
	} */

	if p.RequestCollector != nil {
		handler = p.RequestCollector.Middleware(handler)
	}

	if p.RateLimiter != nil {
		handler = p.RateLimiter.Middleware(handler)
	}

	if p.Subscription != nil {
		handler = p.Subscription.Middleware()(handler)
	}

	// waiter is last before otel tracing
	waiter := util.NewWaiter()
	handler = waiter.Middleware(handler)

	// otel tracing
	traceHandler := func(next jrpc.Handler) jrpc.Handler {
		tracer := otel.Tracer("jrpc")

		fn := jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, req *jsonrpc.Request) {
			chain, _ := subctx.GetChain(req.Context())

			ctx, span := tracer.Start(req.Context(), req.Method,
				trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(
					attribute.String("method", req.Method),
					attribute.String("params", string(req.Params)),
					attribute.String("chain", chain)))
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
	}

	handler = traceHandler(handler)

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

		// the primary mount point, where we grab the chain and inject it into the request context
		r.Mount("/{chain}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			chainString := chi.URLParam(r, "chain")
			r = r.WithContext(subctx.WithChain(r.Context(), chainString))
			serverHandler.ServeHTTP(w, r)
		}))

		// health check
		r.Mount("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		}))
		// TODO: eventually stats/dashboard will be here.
	}
	return
}
