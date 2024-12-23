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

	"gfx.cafe/gfx/venn/lib/util"
)

type Blockstores = util.Multichain[blockstore.Store]
type Headstores = util.Multichain[headstore.Store]

type Params struct {
	fx.In

	Lc fx.Lifecycle

	Log    *slog.Logger
	Chains map[string]*config.Chain

	// Cassblock   *cassblock.Cassblock `optional:"true"`
	Rediblock  rediblock.Rediblocks `optional:"true"`
	Chainblock chainblock.Chainblocks

	Redihead redihead.Rediheads
}

type Result struct {
	fx.Out

	Blockstores Blockstores
	Headstores  Headstores
}

func New(p Params) (r Result, err error) {
	r.Blockstores, err = util.MakeMultichain(
		p.Chains,
		func(chain *config.Chain) (blockstore.Store, error) {
			compoundStore := blockstore.NewCompoundStore(p.Log)
			compoundStore.AddStore(
				"lru",
				blockstore.NewLruStore(16),
			)
			if p.Rediblock != nil {
				rb, err := util.GetChain(chain.Name, p.Rediblock)
				if err != nil {
					return nil, err
				}
				compoundStore.AddStore("rediblock", rb)
			}
			/*
				if p.Cassblock != nil {
					compoundStore.AddStore("cassblock", p.Cassblock)
				}
			*/
			cb, err := util.GetChain(chain.Name, p.Chainblock)
			if err != nil {
				return nil, err
			}
			compoundStore.AddStore("blockgetter", cb)

			return blockstore.NewDeduper(compoundStore), nil
		},
	)
	if err != nil {
		return
	}
	r.Headstores, err = util.MakeMultichain(
		p.Chains,
		func(chain *config.Chain) (headstore.Store, error) {
			if p.Redihead != nil {
				return util.GetChain(chain.Name, p.Redihead)
			}

			return headstore.NewAtomic(), nil
		},
	)
	return
}
