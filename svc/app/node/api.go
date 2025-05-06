package node

import (
	"context"
	"fmt"
	"net/http"

	"gfx.cafe/gfx/venn/svc/middlewares/forger"

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

	// head following for even faster access to the latest block.
	Stalker *stalker.Stalker

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
	handler := p.Clusters.Middleware(nil)

	handler = p.Cacher.Middleware(handler)

	handler = p.Stalker.Middleware(handler)

	handler = (&forger.Forger{}).Middleware(handler)

	handler = p.Subcenter.Middleware(handler)

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
	}

	handler = traceHandler(handler)

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
