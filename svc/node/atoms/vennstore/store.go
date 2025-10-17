package vennstore

import (
	"log/slog"

	"github.com/gfx-labs/venn/lib/config"
	"github.com/gfx-labs/venn/lib/stores/blockstore"

	"go.uber.org/fx"

	"github.com/gfx-labs/venn/svc/node/stores/vennstores/chainblock"
	"github.com/gfx-labs/venn/svc/node/stores/vennstores/rediblock"
)

type Params struct {
	fx.In

	Lc fx.Lifecycle

	Log    *slog.Logger
	Chains map[string]*config.Chain

	Rediblock  *rediblock.Rediblock `optional:"true"`
	Chainblock *chainblock.Chainblock
}

type Result struct {
	fx.Out

	Blockstore blockstore.Store
}

func New(p Params) (r Result, err error) {
	compoundStore := blockstore.NewCompoundStore(p.Log)
	compoundStore.AddStore(
		"lru",
		blockstore.NewLruStore(2048),
	)
	if p.Rediblock != nil {
		compoundStore.AddStore("rediblock", p.Rediblock)
	}
	compoundStore.AddStore("blockgetter", p.Chainblock)
	r.Blockstore = blockstore.NewSingleFlight(compoundStore)

	return
}
