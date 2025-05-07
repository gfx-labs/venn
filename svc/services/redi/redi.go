package redi

import (
	"context"
	"log/slog"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/redis/rueidis"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
)

type Redis struct {
	c       *redis.Client
	cfg     config.Redis
	rueidis rueidis.Client
}

type RedisParams struct {
	fx.In

	Config config.Redis
	Log    *slog.Logger
	Lc     fx.Lifecycle
}

type RedisResult struct {
	fx.Out

	Redis *Redis
}

func New(params RedisParams) (res RedisResult, err error) {
	r := &Redis{
		cfg: params.Config,
	}
	if params.Config.URI == "embedded" || params.Config.URI == "" {
		params.Log.Info("running with embedded redis")
		mr := miniredis.NewMiniRedis()
		if err := mr.Start(); err != nil {
			return res, err
		}
		r.c = redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
		})
		r.rueidis, err = rueidis.NewClient(rueidis.ClientOption{
			InitAddress:  []string{mr.Addr()},
			DisableCache: true,
		})

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
				if err := r.c.Close(); err != nil {
					return err
				}
				mr.Close()
				return nil
			},
		})
	} else {
		opts, err := redis.ParseURL(string(params.Config.URI))
		if err != nil {
			return res, err
		}
		rudidisOpts, err := rueidis.ParseURL(string(params.Config.URI))
		if err != nil {
			return res, err
		}
		params.Log.Info("connecting to redis", "addr", opts.Addr, "user", opts.Username)
		r.rueidis, err = rueidis.NewClient(rudidisOpts)
		if err != nil {
			return res, err
		}
		r.c = redis.NewClient(opts)
		params.Lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				r.rueidis.Close()
				return r.c.Close()
			},
		})
	}
	return RedisResult{
		Redis: r,
	}, nil
}

func (r *Redis) R() rueidis.Client {
	return r.rueidis
}
func (r *Redis) C() *redis.Client {
	return r.c
}

var compareAndSwapIfGreaterScript = redis.NewScript(`
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
func (r *Redis) CompareAndSwapIfGreater(ctx context.Context, key string, new int) (int, error) {
	res, err := compareAndSwapIfGreaterScript.Run(
		ctx,
		r.C(),
		[]string{key},
		new,
	).Result()
	if err != nil {
		return 0, err
	}
	return int(res.(int64)), nil
}

func (r *Redis) Namespace() string {
	return r.cfg.Namespace
}
