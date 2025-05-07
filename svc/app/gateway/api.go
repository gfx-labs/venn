package gateway

import (
	"context"
	"fmt"
	"log/slog"
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

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/subctx"
	"gfx.cafe/gfx/venn/lib/util"
)

type Params struct {
	fx.In

	Lc           fx.Lifecycle
	Subscription *subscription.Engine `optional:"true"`
	Endpoints    map[string]*config.EndpointSpec
	Security     *config.Security
	Logger       *slog.Logger

	// head following for even faster access to the latest block.

	TraceProvider *gotel.TraceProvider `optional:"true"`
}

type Result struct {
	fx.Out

	Route func(r chi.Router) `group:"route"`
}

func New(p Params) (r Result, err error) {

	waiter := util.NewWaiter()
	// otel tracing
	traceHandler := func(next jrpc.Handler) jrpc.Handler {
		tracer := otel.Tracer("jrpc")
		fn := jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, req *jsonrpc.Request) {
			path, _ := subctx.GetEndpointPath(req.Context())
			ctx, span := tracer.Start(req.Context(), req.Method,
				trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(
					attribute.String("method", req.Method),
					attribute.String("params", string(req.Params)),
					attribute.String("path", path)))
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

	proxies := make(map[string]map[string]jrpc.Handler)
	for _, endpoint := range p.Endpoints {
		hybridProxy := callcenter.NewHybridProxy(p.Logger, string(endpoint.VennUrl))
		proxies[endpoint.Name] = map[string]jrpc.Handler{}
		p.Logger.Info("registering endpoint", "name", endpoint.Name, "paths", len(endpoint.Paths))
		// per endpoint middleware that are applied to each path handler
		var localMiddleware []func(jrpc.Handler) jrpc.Handler = []func(jrpc.Handler) jrpc.Handler{}
		for _, to := range endpoint.Paths {
			_, ok := proxies[endpoint.Name][to]
			if ok {
				continue
			}
			endpointPathHandler, err := hybridProxy.EndpointHandler(to)
			if err != nil {
				return r, err
			}
			handler := jrpc.Handler(endpointPathHandler)
			for _, m := range localMiddleware {
				handler = m(handler)
			}
			proxies[endpoint.Name][to] = handler
		}
	}

	baseHandler := jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		endpoint, err := subctx.GetEndpointSpec(r.Context())
		if err != nil {
			w.Send(nil, err)
			return
		}
		target, err := subctx.GetEndpointPath(r.Context())
		if err != nil {
			w.Send(nil, err)
			return
		}
		endpointProxies, ok := proxies[endpoint.Name]
		if !ok {
			w.Send(nil, fmt.Errorf("no endpoint proxies for %s", endpoint.Name))
			return
		}
		targetProxy, ok := endpointProxies[target]
		if !ok {
			w.Send(nil, fmt.Errorf("no target proxy for %s", target))
			return
		}
		targetProxy.ServeRPC(w, r)
		return
	})

	// global middleware that apply before endpoint routing goes here
	var globalMiddlewares []func(jrpc.Handler) jrpc.Handler = []func(jrpc.Handler) jrpc.Handler{
		p.Subscription.Middleware(),
		waiter.Middleware,
		traceHandler,
	}

	jrpcHandler := jrpc.Handler(baseHandler)
	for _, m := range globalMiddlewares {
		jrpcHandler = m(jrpcHandler)
	}

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
	serverHandler := codecs.HttpWebsocketHandler(baseHandler, nil)

	// mount the http server
	r.Route = func(r chi.Router) {
		r.Use(otelchi.Middleware("gateway", otelchi.WithChiRoutes(r), otelchi.WithFilter(
			func(r *http.Request) bool {
				return r.Header.Get("upgrade") == ""
			})))
		for _, endpoint := range p.Endpoints {
			for from, to := range endpoint.Paths {
				r.Mount("/"+endpoint.Name+"/"+from, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r = r.WithContext(subctx.WithEndpointSpec(r.Context(), endpoint))
					r = r.WithContext(subctx.WithEndpointPath(r.Context(), to))
					serverHandler.ServeHTTP(w, r)
				}))
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
