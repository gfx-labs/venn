package vennstore

import (
	"log/slog"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/stores/blockstore"
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/svc/stores/headstores/redihead"

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

	Redihead *redihead.Redihead
}

type Result struct {
	fx.Out

	Blockstore blockstore.Store
	Headstore  headstore.Store
}

func New(p Params) (r Result, err error) {
	compoundStore := blockstore.NewCompoundStore(p.Log)
	compoundStore.AddStore(
		"lru",
		blockstore.NewLruStore(128),
	)
	if p.Rediblock != nil {
		compoundStore.AddStore("rediblock", p.Rediblock)
	}
	/*
		if p.Cassblock != nil {
			compoundStore.AddStore("cassblock", p.Cassblock)
		}
	*/
	compoundStore.AddStore("blockgetter", p.Chainblock)
	r.Blockstore = blockstore.NewSingleFlight(compoundStore)

	// set headstore
	if p.Redihead != nil {
		r.Headstore = p.Redihead
	} else {
		r.Headstore = headstore.NewAtomic()
	}
	return
}
