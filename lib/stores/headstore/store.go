package headstore

import (
	"context"
	"github.com/gfx-labs/venn/lib/config"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Store interface {
	Get(ctx context.Context, chain *config.Chain) (hexutil.Uint64, error)
	Put(ctx context.Context, chain *config.Chain, head hexutil.Uint64) (prev hexutil.Uint64, err error)
	On(chain *config.Chain) (<-chan hexutil.Uint64, func())
}
