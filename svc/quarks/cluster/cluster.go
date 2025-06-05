package cluster

import (
	"context"
	"io"
	"log/slog"
	"time"

	"gfx.cafe/gfx/venn/lib/subctx"
	"gfx.cafe/open/jrpc/contrib/codecs/websocket"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"

	"gfx.cafe/gfx/venn/svc/middlewares/blockLookBack"
	"gfx.cafe/gfx/venn/svc/stores/headstores/redihead"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/contrib/codecs/http"
	"go.uber.org/fx"
	"golang.org/x/time/rate"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/svc/services/prom"
)

// RemoteMiddlewares holds all middleware instances for a specific remote
type RemoteMiddlewares struct {
	InputData     *callcenter.InputData
	Collector     *callcenter.Collector
	Logger        *callcenter.Logger
	Backer        *callcenter.Backer
	Validator     *callcenter.Validator
	Doctor        *callcenter.Doctor
	RateLimiter   *callcenter.Ratelimiter
	Filterer      *callcenter.Filterer
	BlockLookBack callcenter.Middleware // blockLookBack returns a Middleware interface
}

type Clusters struct {
	Remotes map[string]*callcenter.Cluster

	// Middleware instances stored by chain name, then by remote name
	middlewares map[string]map[string]*RemoteMiddlewares
}

type Params struct {
	fx.In

	Log        *slog.Logger
	Prometheus *prom.Prometheus
	Chains     map[string]*config.Chain
	HeadStore  *redihead.Redihead
	Lc         fx.Lifecycle
}

type Result struct {
	fx.Out

	Clusters *Clusters
}

func New(params Params) (r Result, err error) {
	r.Clusters = &Clusters{
		Remotes:     make(map[string]*callcenter.Cluster),
		middlewares: make(map[string]map[string]*RemoteMiddlewares),
	}
	for _, chain := range params.Chains {
		cluster := callcenter.NewCluster()
		r.Clusters.Remotes[chain.Name] = cluster
		// Initialize the nested map for this chain
		r.Clusters.middlewares[chain.Name] = make(map[string]*RemoteMiddlewares)
		for _, cfg := range chain.Remotes {
			cfg := cfg
			toclose := make([]io.Closer, 0)
			params.Lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					// Initialize middleware storage
					mw := &RemoteMiddlewares{}
					r.Clusters.middlewares[chain.Name][cfg.Name] = mw

					// Create base proxier
					proxier := callcenter.NewProxier(func(ctx context.Context) (jrpc.Conn, error) {
						c, err := jrpc.DialContext(ctx, string(cfg.Url))
						if err != nil {
							return nil, err
						}
						switch cc := c.(type) {
						case *http.Client:
							for key, value := range cfg.Headers {
								cc.SetHeader(key, value)
							}
						case *websocket.Client:
							for key, value := range cfg.Headers {
								cc.SetHeader(key, value)
							}
						}
						return c, nil
					})
					toclose = append(toclose, proxier)

					mw.InputData = &callcenter.InputData{}

					mw.Collector = callcenter.NewCollector(
						chain.Name,
						cfg.Name,
						params.Prometheus,
					)

					mw.Logger = callcenter.NewLogger(
						params.Log.With("remote", cfg.Name, "chain", chain.Name),
					)

					mw.Backer = callcenter.NewBacker(
						params.Log.With("remote", cfg.Name, "chain", chain.Name),
						cfg.RateLimitBackoff.Duration,
						cfg.ErrorBackoffMin.Duration,
						cfg.ErrorBackoffMax.Duration,
					)

					mw.Validator = callcenter.NewValidator(
						max(time.Minute, time.Duration(float64(time.Second)*2*chain.BlockTimeSeconds)),
					)

					mw.Doctor = callcenter.NewDoctor(
						params.Log.With("remote", cfg.Name, "chain", chain.Name),
						chain.Id,
						cfg.HealthCheckIntervalMin.Duration,
						cfg.HealthCheckIntervalMax.Duration,
					)

					if cfg.RateLimit != nil {
						mw.RateLimiter = callcenter.NewRatelimiter(
							rate.Limit(cfg.RateLimit.EventsPerSecond),
							cfg.RateLimit.Burst,
						)
					} else {
						// default values of 50/100
						mw.RateLimiter = callcenter.NewRatelimiter(
							50,
							100,
						)
					}

					methods := make(map[string]bool)
					for _, filter := range cfg.ParsedFilters {
						for method, ok := range filter.Methods {
							methods[method] = ok
						}
					}
					mw.Filterer = callcenter.NewFilterer(methods)

					if cfg.MaxBlockLookBack > 0 {
						mw.BlockLookBack = blockLookBack.New(cfg, params.HeadStore)
					}

					// Now build the middleware chain
					var remote jrpc.Handler = proxier

					if mw.InputData != nil {
						remote = mw.InputData.Middleware(remote)
					}

					remote = mw.Collector.Middleware(remote)
					remote = mw.Logger.Middleware(remote)
					remote = mw.Backer.Middleware(remote)
					remote = mw.Validator.Middleware(remote)
					remote = mw.Doctor.Middleware(remote)
					remote = mw.RateLimiter.Middleware(remote)
					remote = mw.Filterer.Middleware(remote)

					if mw.BlockLookBack != nil {
						remote = mw.BlockLookBack.Middleware(remote)
					}

					cluster.Add(cfg.Priority, remote)

					return nil
				},
				OnStop: func(_ context.Context) error {
					for _, v := range toclose {
						_ = v.Close()
					}
					return nil
				},
			})
		}
	}
	return
}

func (T *Clusters) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		chain, err := subctx.GetChain(r.Context())
		if err != nil {
			_ = w.Send(nil, err)
			return
		}
		remote, ok := T.Remotes[chain.Name]
		if !ok {
			if next != nil {
				next.ServeRPC(w, r)
			} else {
				w.Send(nil, jsonrpc.NewInvalidRequestError("chain not supported: "+chain.Name))
			}
			return
		}
		remote.ServeRPC(w, r)
	})
}
