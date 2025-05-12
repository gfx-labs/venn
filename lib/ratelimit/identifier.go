package ratelimit

import (
	"context"
	"errors"
	"time"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/redis/rueidis/rueidislimiter"
)

type Identifier struct {
	Endpoint string
	Type     string
	Slug     string

	ExtraCost int
}

func (i *Identifier) String() string {
	return i.String()
}

func (i *Identifier) Key() string {
	return i.Endpoint + ":" + i.Type + ":" + i.Slug
}

type identifierContextKeyType string

var identifierContextKey identifierContextKeyType = "rl_identifier"
var errNoIdentifier = errors.New("no valid ratelimit identifier for request")

func IdentifierFromContext(ctx context.Context) (*Identifier, error) {
	v := ctx.Value(identifierContextKey)
	if v == nil {
		return nil, errNoIdentifier
	}
	val, ok := v.(*Identifier)
	if !ok {
		return nil, errNoIdentifier
	}
	return val, nil
}

func WithIdentifier(idFunc func(r *jrpc.Request) (*Identifier, error)) func(jrpc.Handler) jrpc.Handler {
	return func(next jrpc.Handler) jrpc.Handler {
		return jsonrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
			id, err := idFunc(r)
			if err != nil {
				w.Send(nil, err)
				return
			}
			if id == nil {
				id = &Identifier{
					Type: "nil",
					Slug: "nil",
				}
			}
			r = r.WithContext(context.WithValue(r.Context(), identifierContextKey, id))
			next.ServeRPC(w, r)
		})
	}
}

func RuedisRatelimiter(rl rueidislimiter.RateLimiterClient) func(jrpc.Handler) jrpc.Handler {
	return func(next jrpc.Handler) jrpc.Handler {
		return jsonrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
			id, err := IdentifierFromContext(r.Context())
			if err != nil {
				w.Send(nil, err)
				return
			}
			rateLimitKey := id.Key()
			wait, err := rl.AllowN(r.Context(), rateLimitKey, int64(1+id.ExtraCost))
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
			next.ServeRPC(w, r)
		})
	}
}
