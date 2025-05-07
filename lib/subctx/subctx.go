package subctx

import (
	"context"
	"errors"

	"gfx.cafe/gfx/venn/lib/config"
)

type ContextKeyChainType string

var ErrNoChain = errors.New("no chain specified")
var ErrNoEndpointPath = errors.New("no endpoint path specified")
var ErrNoEndpointSpec = errors.New("no endpoint spec specified")

const (
	ContextKeyChain        ContextKeyChainType = "VennChain"
	ContextKeyInternal     ContextKeyChainType = "VennInternal"
	ContextKeyEndpointPath ContextKeyChainType = "VennEndpointPath"
	ContextKeyEndpointSpec ContextKeyChainType = "VennEndpointSpec"
)

func WithChain(ctx context.Context, chain *config.Chain) context.Context {
	return context.WithValue(ctx, ContextKeyChain, chain)
}

func WithEndpointSpec(ctx context.Context, endpoint *config.EndpointSpec) context.Context {
	return context.WithValue(ctx, ContextKeyEndpointSpec, endpoint)
}

func WithEndpointPath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, ContextKeyEndpointPath, path)
}

func WithInternal(ctx context.Context, internal bool) context.Context {
	return context.WithValue(ctx, ContextKeyInternal, internal)
}

func GetEndpointPath(ctx context.Context) (string, error) {
	val := ctx.Value(ContextKeyEndpointPath)
	valS, ok := val.(string)
	if !ok || valS == "" {
		return "", ErrNoEndpointPath
	}
	return valS, nil
}

func GetEndpointSpec(ctx context.Context) (*config.EndpointSpec, error) {
	val := ctx.Value(ContextKeyEndpointSpec)
	valS, ok := val.(*config.EndpointSpec)
	if !ok || valS == nil {
		return nil, ErrNoEndpointSpec
	}
	return valS, nil
}

func GetChain(ctx context.Context) (*config.Chain, error) {
	val := ctx.Value(ContextKeyChain)
	valS, ok := val.(*config.Chain)
	if !ok || valS == nil {
		return nil, ErrNoChain
	}
	return valS, nil
}

func IsInternal(ctx context.Context) bool {
	val := ctx.Value(ContextKeyInternal)
	return val.(bool)
}
