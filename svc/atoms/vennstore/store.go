package vennstore

import (
	"log/slog"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/stores/blockstore"

	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/svc/stores/vennstores/chainblock"
	"gfx.cafe/gfx/venn/svc/stores/vennstores/rediblock"
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
