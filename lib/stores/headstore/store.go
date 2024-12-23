package headstore

import (
	"context"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Store interface {
	Get(ctx context.Context) (hexutil.Uint64, error)
	Put(ctx context.Context, head hexutil.Uint64) error
	On() (<-chan hexutil.Uint64, func())
}
