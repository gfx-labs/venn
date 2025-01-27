package cluster

import (
	"context"
	"log/slog"
	"time"

	"gfx.cafe/gfx/venn/svc/middlewares/blockLookBack"
	"gfx.cafe/gfx/venn/svc/stores/headstores/redihead"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/contrib/codecs/http"
	"go.uber.org/fx"
	"golang.org/x/time/rate"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/util"
	"gfx.cafe/gfx/venn/svc/services/prom"
)

type Clusters = util.Multichain[callcenter.Remote]

type Params struct {
	fx.In

	Log        *slog.Logger
	Prometheus *prom.Prometheus
	Chains     map[string]*config.Chain
	HeadStores util.Multichain[*redihead.Redihead]
	Lc         fx.Lifecycle
}

type Result struct {
	fx.Out

	Clusters Clusters
}

func New(params Params) (r Result, err error) {
	r.Clusters, err = util.MakeMultichain(
		params.Chains,
		func(chain *config.Chain) (callcenter.Remote, error) {
			cluster := callcenter.NewCluster()

			for _, cfg := range chain.Remotes {
				cfg := cfg

				params.Lc.Append(fx.Hook{
					OnStart: func(ctx context.Context) error {
						var remote callcenter.Remote

						remote = callcenter.NewProxier(func(ctx context.Context) (jrpc.Conn, error) {
							c, err := jrpc.DialContext(ctx, string(cfg.Url))
							if err != nil {
								return nil, err
							}
							switch cc := c.(type) {
							case *http.Client:
								for key, value := range cfg.Headers {
									cc.SetHeader(key, value)
								}
							}
							return c, nil
						})

						/*remote = callcenter.NewPooler(
							func(ctx context.Context) (callcenter.Remote, error) {
								return callcenter.NewProxier(ctx, func(ctx context.Context) (jrpc.Conn, error) {
									return jrpc.DialContext(ctx, string(cfg.Url))
								})
							},
							16,
						)*/

						if cfg.SendDataAndInput {
							remote = callcenter.NewInputData(
								remote,
							)
						}

						remote = callcenter.NewCollector(
							remote,
							chain.Name,
							cfg.Name,
							params.Prometheus,
						)

						remote = callcenter.NewLogger(
							remote,
							params.Log.With("remote", cfg.Name, "chain", cfg.Chain.Name),
						)

						remote = callcenter.NewBacker(
							remote,
							params.Log.With("remote", cfg.Name, "chain", cfg.Chain.Name),
							cfg.ParsedRateLimitBackoff,
							cfg.ParsedErrorBackoffMin,
							cfg.ParsedErrorBackoffMax,
						)

						remote = callcenter.NewValidator(
							remote,
							max(time.Minute, time.Duration(float64(time.Second)*2*cfg.Chain.BlockTimeSeconds)),
						)

						remote = callcenter.NewDoctor(
							remote,
							params.Log.With("remote", cfg.Name, "chain", cfg.Chain.Name),
							cfg.Chain.Id,
							cfg.ParsedHealthCheckIntervalMin,
							cfg.ParsedHealthCheckIntervalMax,
						)

						if cfg.RateLimit != nil {
							remote = callcenter.NewRatelimiter(
								remote,
								rate.Limit(cfg.RateLimit.EventsPerSecond),
								cfg.RateLimit.Burst,
							)
						}

						methods := make(map[string]bool)
						for _, filter := range cfg.ParsedFilters {
							for method, ok := range filter.Methods {
								methods[method] = ok
							}
						}

						remote = callcenter.NewFilterer(
							remote,
							methods,
						)

						if cfg.MaxBlockLookBack > 0 {
							// mount the "proxy" / middleware in front of the remote
							remote = blockLookBack.New(cfg, params.HeadStores[cfg.Chain.Name], remote)
						}

						cluster.Add(cfg.Priority, remote)

						return nil
					},
					OnStop: func(_ context.Context) error {
						_ = cluster.Close()
						return nil
					},
				})
			}

			return cluster, nil
		},
	)
	return
}
