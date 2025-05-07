package ratelimit

import (
	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
)

type Identifier struct {
	Type string
	Slug string
}

func (i *Identifier) String() string {
	return i.Type + ":" + i.Slug
}

type identifierContextKey string

func WithIdentifier(idFunc func(r *jrpc.Request) *Identifier) func(jrpc.Handler) jrpc.Handler {
	return func(next jrpc.Handler) jrpc.Handler {
		return jsonrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
			id := idFunc(r)
			if id == nil {
				id = &Identifier{
					Type: "nil",
					Slug: "nil",
				}
			}
		})
	}
}
