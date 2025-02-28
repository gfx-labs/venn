package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/dranikpg/gtrs"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/svc/services/redi"

	"gfx.cafe/gfx/venn/lib/config"
)

type Limiter struct {
	config  *config.RateLimit
	redis   *redi.Redis
	dir     *Directory
	log     *slog.Logger
	entries gtrs.Stream[Entry]

	streamKey string
}

type LimiterParams struct {
	fx.In

	Config *config.RateLimit
	Log    *slog.Logger
	Redis  *redi.Redis `optional:"true"`

	Lc fx.Lifecycle
}

type LimiterResult struct {
	fx.Out

	Limiter *Limiter
}

func New(params LimiterParams) LimiterResult {
	if params.Config == nil {
		return LimiterResult{}
	}
	if params.Redis == nil {
		params.Log.Info("no redis configured, rate limiter disabled")
		return LimiterResult{}
	}

	limiter := &Limiter{
		config: params.Config,
		dir:    NewDirectory(),
		log:    params.Log,
		// TODO: make this configurable
		streamKey: params.Redis.Namespace() + "ratelimit:actions",
		redis:     params.Redis,
	}

	ctx, cn := context.WithCancel(context.Background())
	params.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			limiter.entries = gtrs.NewStream[Entry](params.Redis.R(), limiter.streamKey, &gtrs.Options{
				MaxLen: 1280,
			})

			cs := gtrs.NewConsumer[Entry](ctx, params.Redis.R(), map[string]string{
				limiter.streamKey: "$",
			})

			// first catch up
			params.Log.Info("catching up with ratelimit log")
			err := limiter.reconcileBans(ctx)
			if err != nil {
				return err
			}
			params.Log.Info("starting ratelimit listener")
			go func() {
				for msg := range cs.Chan() {
					limiter.processAction(msg.ID, &msg.Data)
				}
			}()
			// then start
			return nil
		},
		OnStop: func(_ context.Context) error {
			cn()
			return nil
		},
	})

	return LimiterResult{
		Limiter: limiter,
	}
}

func (rl *Limiter) reconcileBans(ctx context.Context) error {
	// read the bans in the past hour. should be basically instant.
	now := time.Now()
	res, err := rl.entries.Range(ctx,
		fmt.Sprintf("%d-0", now.Add(-1*time.Hour).UnixMilli()),
		fmt.Sprintf("%d-0", now.UnixMilli()),
	)
	if err != nil {
		return err
	}
	for _, msg := range res {
		dat := msg.Data
		rl.processAction(msg.ID, &dat)
	}
	return nil
}

func (rl *Limiter) processAction(id string, e *Entry) {
	rl.log.Warn(
		"ratelimit action enforced",
		"id", id,
		"action", e.Action,
		"user", e.User,
		"until", e.Until)
	switch e.Action {
	case "ban":
		rl.dir.Ban(e)
	default:
		rl.log.Error(
			"unrecognized redis action",
			"id", id,
			"action", e.Action,
			"user", e.User,
			"until", e.Until)
	}
}

func (rl *Limiter) Middleware(h jrpc.Handler) jrpc.Handler {
	return jsonrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		ipAddress := r.Peer.HTTP.RemoteAddr
		if wait := rl.CheckLimit(ipAddress); wait > 0 {
			w.Send(nil, &jsonrpc.JsonError{
				Code:    429,
				Message: "Rate Limit Hit",
				Data: map[string]any{
					"Wait": wait,
				},
			})
			return
		}
		go func() {
			err := rl.ApplyUsage(r.Context(), 1, ipAddress)
			if err != nil {
				rl.log.Error("Failed to apply ratelimit", "addr", ipAddress, "err", err)
				return
			}
		}()
		h.ServeRPC(w, r)
	})
}

func (rl *Limiter) ApplyUsage(ctx context.Context, amount int, tags ...string) error {
	// example.
	// bucketSize=200
	// bucketDrip=100
	// bucketCycleTimeSeconds=10
	// the bucket starts with 200 tokens
	// every 10 seconds, 100 tokens will be added to the bucket
	// this means a sustained rps of 10/s over 10 seconds, with burst of 20/s available.

	key := strings.Join(tags, ":")
	// TODO: allow this to be configured
	_, err := LuaAllowN.Run(ctx, rl.redis.C(), []string{
		key,
		"banned:" + key,
		rl.streamKey,
	},
		rl.config.BucketSize,
		rl.config.BucketDrip,
		rl.config.BucketCycleSeconds,
		amount,
	).Result()
	return err
}

func (rl *Limiter) CheckLimit(tags ...string) (remaining time.Duration) {
	key := strings.Join(tags, ":")
	until := rl.dir.Check(key)
	remaining = time.Until(until)
	return remaining
}
