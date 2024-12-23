package handler

import (
	"context"
	"fmt"
	"gfx.cafe/open/jrpc/contrib/jrpcutil"
	"gfx.cafe/util/go/gotel"
	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"net/http"

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
	Forgers          forger.Forgers
	Stalkers         stalker.Stalkers
	Cachers          cacher.Cachers
	Clusters         cluster.Clusters
	Subcenters       subcenter.Subcenters
	TraceProvider    *gotel.TraceProvider `optional:"true"`
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

	var handler jrpc.Handler = jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, req *jsonrpc.Request) {
		chain, err := subctx.GetChain(req.Context())
		if err != nil {
			_ = w.Send(nil, err)
			return
		}
		provider, err := util.GetChain(chain, r.Providers)
		if err != nil {
			_ = w.Send(nil, err)
			return
		}

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

	waiter := util.NewWaiter()
	handler = waiter.Middleware(handler)

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

	// mount the blockland middleware
	// create the jrpc server
	p.Lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			if err := waiter.Wait(ctx); err != nil {
				return err
			}
			return nil
		},
	})
	serverHandler := codecs.HttpWebsocketHandler(handler, nil)
	r.Route = func(r chi.Router) {
		r.Use(otelchi.Middleware("venn", otelchi.WithChiRoutes(r), otelchi.WithFilter(
			func(r *http.Request) bool {
				return r.Header.Get("upgrade") == ""
			})))
		r.Mount("/{chain}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			chainString := chi.URLParam(r, "chain")
			r = r.WithContext(subctx.WithChain(r.Context(), chainString))
			serverHandler.ServeHTTP(w, r)
		}))
		r.Mount("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		}))
		// TODO: eventually stats/dashboard will be here.
	}
	return
}
