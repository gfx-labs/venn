package redi

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidiscompat"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
)

type Redis struct {
	c   rueidis.Client
	cfg *config.Redis
}

type RedisParams struct {
	fx.In

	Config *config.Redis `optional:"true"`
	Log    *slog.Logger
	Lc     fx.Lifecycle
}

type RedisResult struct {
	fx.Out

	Redis *Redis `optional:"true"`
}

func New(params RedisParams) (res RedisResult, err error) {
	if params.Config == nil {
		params.Log.Info("redis disabled", "reason", "no redis config block")
		return
	}
	r := &Redis{
		cfg: params.Config,
	}
	if params.Config.URI == "embedded" || params.Config.URI == "" {
		params.Log.Info("running with embedded redis")
		mr := miniredis.NewMiniRedis()
		if err := mr.Start(); err != nil {
			return res, err
		}
		r.c, err = rueidis.NewClient(rueidis.ClientOption{
			ForceSingleClient: true,
			InitAddress: []string{
				mr.Addr(),
			},
			DisableCache: true,
		})
		if err != nil {
			return res, err
		}

		params.Lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				go func() {
					prev := time.Now()
					ticker := time.NewTicker(1 * time.Second)
					defer ticker.Stop()
					for {
						select {
						case next := <-ticker.C:
							mr.FastForward(next.Sub(prev))
							prev = next
						case <-mr.Ctx.Done():
							return
						}
					}
				}()
				return nil
			},
			OnStop: func(_ context.Context) error {
				r.c.Close()
				mr.Close()
				return nil
			},
		})
	} else {
		opts, err := rueidis.ParseURL(string(params.Config.URI))
		if err != nil {
			return res, err
		}
		params.Log.Info("connecting to redis", "addr", opts.InitAddress, "user", opts.Username)
		r.c, err = rueidis.NewClient(opts)
		if err != nil {
			return res, err
		}
		params.Lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				r.c.Close()
				return nil
			},
		})
	}
	return RedisResult{
		Redis: r,
	}, nil
}

func (r *Redis) C() rueidiscompat.Cmdable {
	return rueidiscompat.NewAdapter(r.c)
}

func (r *Redis) R() rueidis.Client {
	return r.c
}

var compareAndSwapIfGreaterScript = rueidis.NewLuaScript(`
redis.replicate_commands()

local old = tonumber(redis.call('GET', KEYS[1]))
if old == nil then
	old = 0
end
if tonumber(ARGV[1]) > old then
	redis.call('SET', KEYS[1], ARGV[1])
end
return old
`)

// CompareAndSwapIfGreater sets the value at key to new if new is greater. Returns the old value.
func (r *Redis) CompareAndSwapIfGreater(ctx context.Context, key string, next int) (int, error) {
	res, err := compareAndSwapIfGreaterScript.Exec(
		ctx,
		r.R(),
		[]string{key},
		[]string{strconv.Itoa(next)},
	).AsInt64()
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

func (r *Redis) Namespace() string {
	return r.cfg.Namespace
}
