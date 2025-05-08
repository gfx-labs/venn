package subctx

import (
	"context"
	"errors"
	"gfx.cafe/gfx/venn/lib/config"
)

type ContextKeyChainType string

var ErrNoChain = errors.New("no chain specified")

const (
	ContextKeyChain    ContextKeyChainType = "VennChain"
	ContextKeyInternal ContextKeyChainType = "VennInternal"
)

func WithChain(ctx context.Context, chain *config.Chain) context.Context {
	return context.WithValue(ctx, ContextKeyChain, chain)
}

func WithInternal(ctx context.Context, internal bool) context.Context {
	return context.WithValue(ctx, ContextKeyInternal, internal)
}

func GetChain(ctx context.Context) (*config.Chain, error) {
	val := ctx.Value(ContextKeyChain)
	valS, ok := val.(*config.Chain)
	if !ok || valS == nil {
		return nil, ErrNoChain
	}
	return valS, nil
}

func IsInternal(ctx context.Context) (b bool) {
	if val := ctx.Value(ContextKeyInternal); val != nil {
		b, _ = val.(bool)
	}
	return
}
