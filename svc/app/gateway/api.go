package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"gfx.cafe/util/go/gotel"
	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidislimiter"
	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"inet.af/netaddr"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/contrib/codecs"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"

	jrpcjrpcutil "gfx.cafe/open/jrpc/contrib/jrpcutil"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/ratelimit"
	"gfx.cafe/gfx/venn/lib/subctx"
	"gfx.cafe/gfx/venn/lib/util"
	"gfx.cafe/gfx/venn/svc/services/prom"
	"gfx.cafe/gfx/venn/svc/services/redi"
)

type Params struct {
	fx.In

	Lc           fx.Lifecycle
	Subscription *subscription.Engine `optional:"true"`
	Endpoints    map[string]*config.EndpointSpec
	Security     *config.Security
	Logger       *slog.Logger
	Redi         *redi.Redis

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
	proxies := make(map[string]map[string]jrpc.Handler)
	for _, endpoint := range p.Endpoints {
		hybridProxy := callcenter.NewHybridProxy(p.Logger, string(endpoint.VennUrl))
		proxies[endpoint.Name] = map[string]jrpc.Handler{}
		p.Logger.Info("registering endpoint", "name", endpoint.Name, "paths", len(endpoint.Paths))
		// per endpoint middleware that are applied to each path handler
		var localMiddleware []func(jrpc.Handler) jrpc.Handler = []func(jrpc.Handler) jrpc.Handler{}

		for _, v := range endpoint.Limits.Abuse {
			rc, err := rueidislimiter.NewRateLimiter(rueidislimiter.RateLimiterOption{
				ClientBuilder: func(option rueidis.ClientOption) (rueidis.Client, error) {
					return p.Redi.R(), nil
				},
				KeyPrefix: fmt.Sprintf("%s:gateway:abuse:%s:%s", p.Redi.Namespace(), endpoint.Name, v.Id),
				Limit:     v.Total,
				Window:    v.Window.Duration,
			})
			if err != nil {
				return r, err
			}
			localMiddleware = append(localMiddleware, ratelimit.RuedisRatelimiter(rc))
		}
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
				if m != nil {
					handler = m(handler)
				}
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
	var globalMiddlewares []func(jrpc.Handler) jrpc.Handler = []func(jrpc.Handler) jrpc.Handler{}

	globalMiddlewares = append(globalMiddlewares,
		p.Subscription.Middleware(),
		func(fn jrpc.Handler) jrpc.Handler {
			return jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
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
				label := prom.GatewayRequestLabel{
					Endpoint: endpoint.Name,
					Target:   target,
					Method:   r.Method,
				}
				if strings.HasSuffix(r.Method, "_subscribe") {
					label.Success = true
					prom.Gateway.SubscriptionCreated(label).Inc()
					defer prom.Gateway.SubscriptionClosed(label).Inc()
					fn.ServeRPC(w, r)
					return
				}
				start := time.Now()
				icept := &jrpcutil.Interceptor{}
				defer func() {
					dur := time.Since(start)
					label.Success = icept.Error == nil
					prom.Gateway.RequestLatency(label).Observe(dur.Seconds() * 1000)
				}()
				fn.ServeRPC(icept, r)
				_ = w.Send(icept.Result, icept.Error)
			})
		},
		ratelimit.WithIdentifier(func(r *jrpc.Request) (*ratelimit.Identifier, error) {
			endpoint, err := subctx.GetEndpointSpec(r.Context())
			if err != nil {
				return nil, err
			}
			slug, _, err := net.SplitHostPort(r.Peer.RemoteAddr)
			if err != nil {
				slug = r.Peer.RemoteAddr
			}
			return &ratelimit.Identifier{
				Endpoint: endpoint.Name,
				Type:     "ip",
				Slug:     slug,
			}, nil
		}),
		waiter.Middleware,
		func(next jrpc.Handler) jrpc.Handler {
			tracer := otel.Tracer("jrpc")
			fn := jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, req *jsonrpc.Request) {
				path, _ := subctx.GetEndpointPath(req.Context())
				ctx, span := tracer.Start(req.Context(), req.Method,
					trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(
						attribute.String("method", req.Method),
						attribute.String("params", string(req.Params)),
						attribute.String("path", path)))
				defer span.End()
				ew := &jrpcjrpcutil.ErrorRecorder{
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
		},
	)

	jrpcHandler := jrpc.Handler(baseHandler)
	for _, m := range globalMiddlewares {
		if m != nil {
			jrpcHandler = m(jrpcHandler)
		}
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
	serverHandler := codecs.HttpWebsocketHandler(jrpcHandler, nil)

	b := &netaddr.IPSetBuilder{}
	if p.Security != nil {
		for _, v := range p.Security.TrustedOrigins {
			parsedPrefix, err := netaddr.ParseIPPrefix(v)
			if err != nil {
				return r, fmt.Errorf("invalid trusted origin %s: %w", v, err)
			}
			b.AddPrefix(parsedPrefix)
		}
	}
	ipset, err := b.IPSet()
	if err != nil {
		return r, err
	}
	// mount the http server
	r.Route = func(r chi.Router) {
		if p.Security != nil {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					parsedRemote, err := netaddr.ParseIP(util.HostFromRemoteAddr(r.RemoteAddr))
					// if remote is in the trusted ipset, trust the headers that come from it
					if err == nil && ipset.Contains(parsedRemote) {
						for _, h := range p.Security.TrustedIpHeaders {
							val := r.Header.Get(h)
							if val != "" && net.ParseIP(val) != nil {
								r.RemoteAddr = val
								break
							}
						}
					}
					next.ServeHTTP(w, r)
				})
			})
		}
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
