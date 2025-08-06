package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"go4.org/netipx"

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
	"gfx.cafe/open/jrpc/contrib/jmux"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"

	jrpcjrpcutil "gfx.cafe/open/jrpc/contrib/jrpcutil"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/ratelimit"
	"gfx.cafe/gfx/venn/lib/subctx"
	"gfx.cafe/gfx/venn/lib/util"
	"gfx.cafe/gfx/venn/lib/util/origin"
	"gfx.cafe/gfx/venn/svc/gateway/quarks/telemetry"
	"gfx.cafe/gfx/venn/svc/shared/services/prom"
	"gfx.cafe/gfx/venn/svc/shared/services/redi"
)

type Params struct {
	fx.In

	Lc           fx.Lifecycle
	Subscription *subscription.Engine `optional:"true"`
	Endpoint     *config.EndpointSpec
	Security     *config.Security
	Logger       *slog.Logger
	Redi         *redi.Redis

	Telemetry *telemetry.Telemetry

	// head following for even faster access to the latest block.

	TraceProvider *gotel.TraceProvider `optional:"true"`
}

type Result struct {
	fx.Out

	Route func(r chi.Router) `group:"route"`
}

func createBaseHandler(p Params) (jrpc.Handler, error) {
	endpointProxies := make(map[string]jrpc.Handler)
	hybridProxy := callcenter.NewHybridProxy(p.Logger, string(p.Endpoint.VennUrl))
	for _, to := range p.Endpoint.Paths {
		_, ok := endpointProxies[to]
		if ok {
			continue
		}
		endpointPathHandler, err := hybridProxy.EndpointHandler(to)
		if err != nil {
			return nil, err
		}
		endpointProxies[to] = jrpc.Handler(endpointPathHandler)
	}
	baseHandler := jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		target, err := subctx.GetEndpointPath(r.Context())
		if err != nil {
			w.Send(nil, err)
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
	return baseHandler, nil

}

func New(p Params) (r Result, err error) {
	maxRequestBodySize := 5 * 1024 * 1024

	mux := jmux.NewMux()

	// subscription middleware
	mux.Use(p.Subscription.Middleware())

	mux.Use( // make sure request body is not larger than 5mb
		func(next jrpc.Handler) jrpc.Handler {
			return jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, req *jsonrpc.Request) {
				// TODO: make this configurable
				if len(req.Params) > maxRequestBodySize {
					w.Send(nil, fmt.Errorf("request body too large"))
					return
				}
				next.ServeRPC(w, req)
			})
		})

	// whitelist methods
	whitelist := map[string]struct{}{}
	for _, v := range p.Endpoint.Methods {
		whitelist[v] = struct{}{}
	}
	// Method validation middleware
	mux.Use(util.MethodValidationMiddleware())

	// Whitelist validation
	mux.Use(func(next jrpc.Handler) jrpc.Handler {
		return jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
			if len(whitelist) == 0 {
				next.ServeRPC(w, r)
				return
			}
			if _, ok := whitelist[r.Method]; !ok {
				w.Send(nil, fmt.Errorf("method not allowed"))
				return
			}
			next.ServeRPC(w, r)
		})
	})

	// tracing
	mux.Use(func(next jrpc.Handler) jrpc.Handler {
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
	})

	mux.Use(ratelimit.WithIdentifier(func(r *jrpc.Request) (*ratelimit.Identifier, error) {
		endpoint, err := subctx.GetEndpointSpec(r.Context())
		if err != nil {
			return nil, err
		}
		if r.Peer.HTTP != nil {
			r.Peer.RemoteAddr = r.Peer.HTTP.RemoteAddr
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
	}))

	for _, v := range p.Endpoint.Limits.Abuse {
		rc, err := rueidislimiter.NewRateLimiter(rueidislimiter.RateLimiterOption{
			ClientBuilder: func(option rueidis.ClientOption) (rueidis.Client, error) {
				return p.Redi.R(), nil
			},
			KeyPrefix: fmt.Sprintf("%s:gateway:abuse:%s:%s", p.Redi.Namespace(), p.Endpoint.Name, v.Id),
			Limit:     v.Total,
			Window:    v.Window.Duration,
		})
		if err != nil {
			return r, err
		}

		mux.Use(ratelimit.RuedisRatelimiter(rc))
	}

	mux.Use(func(fn jrpc.Handler) jrpc.Handler {
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
			id, err := ratelimit.IdentifierFromContext(r.Context())
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
			icept := &jrpcjrpcutil.ErrorRecorder{
				ResponseWriter: w,
			}
			defer func() {
				dur := time.Since(start)
				label.Success = icept.Error() == nil
				prom.Gateway.RequestLatency(label).Observe(dur.Seconds() * 1000)
				lvl := slog.LevelInfo
				extra := []any{
					"endpoint", endpoint.Name,
					"target", target,
					"method", r.Method,
					"transport", r.Peer.Transport,
					"params", string(r.Params),
					"limit_key", id.Key(),
					"duration", dur,
				}
				if icept.Error() != nil {
					lvl = slog.LevelError
					extra = append(extra, "err", icept.Error())
				}
				p.Logger.Log(context.Background(), lvl, "request",
					extra...,
				)
			}()
			fn.ServeRPC(icept, r)
		})
	})
	mux.Use(func(next jrpc.Handler) jrpc.Handler {
		return jsonrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
			id, err := ratelimit.IdentifierFromContext(r.Context())
			if err != nil {
				w.Send(nil, err)
				return
			}
			entry := &telemetry.Entry{
				Timestamp: time.Now(),
				UsageKey:  id.Key(),
				Method:    r.Method,
				Params:    r.Params,
				Metadata:  map[string]any{},
				Extra:     map[string]any{},
				RequestID: getTraceID(r.Context()),
			}
			next.ServeRPC(w, r)
			entry.Duration = time.Since(entry.Timestamp)

			err = p.Telemetry.Publish(r.Context(), entry)
			if err != nil {
				w.Send(nil, err)
				return
			}
		})
	})

	baseHandler, err := createBaseHandler(p)
	if err != nil {
		return r, err
	}

	mux.Handle("*", baseHandler)

	// bind the jrpc handler to a http+websocket codec to host on the http server
	serverHandler := codecs.HttpWebsocketHandler(mux, []string{"*"})

	b := &netipx.IPSetBuilder{}
	if p.Security != nil {
		for _, v := range p.Security.TrustedOrigins {
			parsedPrefix, err := netip.ParsePrefix(v)
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
					parsedRemote, err := netip.ParseAddr(util.HostFromRemoteAddr(r.RemoteAddr))
					// if remote is in the trusted ipset, trust the headers that come from it
					if err == nil && ipset.Contains(parsedRemote) {
						for _, h := range p.Security.TrustedIpHeaders {
							val := r.Header.Get(h)
							p.Logger.Debug("checking ip header", "header", h, "value", val)
							if val != "" && net.ParseIP(val) != nil {
								r.RemoteAddr = val
								break
							}
						}
					} else {
						p.Logger.Debug("got request from untrusted remote", "remote", r.RemoteAddr)
					}
					if len(p.Security.AllowedOrigins) > 0 {
						o := r.Header.Get("Origin")
						matched := false
						for _, v := range p.Security.AllowedOrigins {
							match, err := origin.Match(o, v)
							if err != nil {
								http.Error(w, "invalid origin", http.StatusForbidden)
								return
							}
							if match {
								matched = true
								break
							}
						}
						if !matched {
							http.Error(w, "origin not allowed", http.StatusForbidden)
							return
						}
					}
					next.ServeHTTP(w, r)
				})
			})
		}
		r.Use(func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// check max size. 5mb for now is more than reasonable.
				// TODO: make this configurable
				if int(r.ContentLength) > maxRequestBodySize {
					w.WriteHeader(http.StatusRequestEntityTooLarge)
					return
				}
				h.ServeHTTP(w, r)
			})
		})
		r.Use(otelchi.Middleware("gateway", otelchi.WithChiRoutes(r), otelchi.WithFilter(
			func(r *http.Request) bool {
				return r.Header.Get("upgrade") == ""
			})))
		for from, to := range p.Endpoint.Paths {
			r.Mount("/"+from, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// add the endpoint spec and target here!
				r = r.WithContext(subctx.WithEndpointPath(r.Context(), to))
				r = r.WithContext(subctx.WithEndpointSpec(r.Context(), p.Endpoint))
				serverHandler.ServeHTTP(w, r)
			}))
		}
		// health check
		r.Mount("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		}))
		// TODO: eventually stats/dashboard will be here.
	}
	return
}

func getTraceID(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		traceID := spanCtx.TraceID()
		return traceID.String()
	}
	return ""
}
