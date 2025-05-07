package ratelimit

import (
	"log/slog"
	"net"
	"strings"
	"time"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidislimiter"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/svc/services/redi"

	"gfx.cafe/gfx/venn/lib/config"
)

type Limiter struct {
	config *config.AbuseLimit
	redis  *redi.Redis
	log    *slog.Logger

	rl rueidislimiter.RateLimiterClient
}

type LimiterParams struct {
	fx.In

	Config *config.AbuseLimit
	Log    *slog.Logger
	Redis  *redi.Redis `optional:"true"`

	Lc fx.Lifecycle
}

type LimiterResult struct {
	fx.Out

	Limiter *Limiter
}

func New(params LimiterParams) (LimiterResult, error) {
	if params.Config == nil {
		return LimiterResult{}, nil
	}
	if params.Redis == nil {
		params.Log.Info("no redis configured, rate limiter disabled")
		return LimiterResult{}, nil
	}
	rLimiter, err := rueidislimiter.NewRateLimiter(rueidislimiter.RateLimiterOption{
		ClientBuilder: func(option rueidis.ClientOption) (rueidis.Client, error) {
			return params.Redis.R(), nil
		},
		// TODO: make this configurable
		KeyPrefix: params.Redis.Namespace() + "ratelimit:actions:",
		Limit:     params.Config.Total,
		Window:    params.Config.Window.Duration,
	})
	if err != nil {
		return LimiterResult{}, err
	}

	limiter := &Limiter{
		rl: rLimiter,
	}
	return LimiterResult{
		Limiter: limiter,
	}, nil
}

func formRateLimitKeyFromIp(r *jsonrpc.Request) (string, error) {
	var rateLimitKey string
	if r.Peer.HTTP == nil {
		return "anonymous", nil
	}
	remoteAddr := r.Peer.HTTP.RemoteAddr
	if strings.Contains(remoteAddr, ":") {
		rateLimitKey, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			return "", err
		}
		return "ip:" + rateLimitKey, nil
	}

	return rateLimitKey, nil
}

func (rl *Limiter) Middleware(h jrpc.Handler) jrpc.Handler {
	return jsonrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		rateLimitKey, err := formRateLimitKeyFromIp(r)
		if err != nil {
			w.Send(nil, &jsonrpc.JsonError{
				Code:    500,
				Message: "Internal Server Error",
				Data: map[string]any{
					"Error": err.Error(),
				},
			})
			return
		}
		wait, err := rl.rl.Allow(r.Context(), rateLimitKey)
		if err != nil {
			w.Send(nil, &jsonrpc.JsonError{
				Code:    500,
				Message: "Internal Server Error",
				Data: map[string]any{
					"Error": err.Error(),
				},
			})
			return
		}
		if !wait.Allowed {
			waitTime := time.UnixMilli(wait.ResetAtMs).Sub(time.Now())
			w.Send(nil, &jsonrpc.JsonError{
				Code:    429,
				Message: "Rate Limit Hit",
				Data: map[string]any{
					"Wait": waitTime / time.Millisecond,
					"Key":  rateLimitKey,
				},
			})
			return
		}
		h.ServeRPC(w, r)
	})
}
