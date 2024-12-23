package subctx

import (
	"context"
	"errors"
)

type ContextKeyChainType string

var ErrNoChain = errors.New("no chain specified")

const (
	ContextKeyChain    ContextKeyChainType = "VennChain"
	ContextKeyInternal ContextKeyChainType = "VennInternal"
)

func WithChain(ctx context.Context, chain string) context.Context {
	return context.WithValue(ctx, ContextKeyChain, chain)
}

func WithInternal(ctx context.Context, internal bool) context.Context {
	return context.WithValue(ctx, ContextKeyInternal, internal)
}

func GetChain(ctx context.Context) (string, error) {
	val := ctx.Value(ContextKeyChain)
	valS, ok := val.(string)
	if !ok || len(valS) == 0 {
		return "", ErrNoChain
	}
	return valS, nil
}

func IsInternal(ctx context.Context) bool {
	val := ctx.Value(ContextKeyInternal)
	return val.(bool)
}
