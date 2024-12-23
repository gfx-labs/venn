package blocktarget

import (
	"context"
	"time"

	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/oracles"
	"gfx.cafe/gfx/venn/lib/oracles/blockoracle"
	"gfx.cafe/gfx/venn/lib/util"
)

type Target struct {
	oracles map[string]oracles.Oracle[int]
}

type Params struct {
	fx.In

	Chains map[string]*config.Chain
}

type Result struct {
	fx.Out

	Output *Target
}

func New(p Params) (r Result, err error) {
	o := &Target{}
	r.Output = o
	o.oracles = make(map[string]oracles.Oracle[int])
	for _, c := range p.Chains {
		chainOracles := []blockoracle.BlockOracle{}
		ttl := max(1*time.Second, time.Duration(float64(c.Network.BlockTimeSeconds)*float64(time.Second)))
		for _, v := range c.HeadOracles {
			baseOracle, err := blockoracle.HttpJsonOracle(string(v.Url), v.CelExpr)
			if err != nil {
				return r, err
			}
			// never ask more than once a second
			composedOracle := oracles.TimeoutOracle(oracles.TtlOracle(
				oracles.SingleFlightOracle(baseOracle), ttl,
				// 5 second timeout for any request
			), 5*time.Second)
			chainOracles = append(chainOracles, composedOracle)
		}
		if len(chainOracles) == 0 {
			// no head oracle for this chain
			continue
		}
		o.oracles[c.Name] = oracles.MaxTrimOracle(chainOracles)
	}
	return
}

func (b *Target) GetHeadForChain(ctx context.Context, chain string) (int, error) {
	oracle, err := util.GetChain(chain, b.oracles)
	if err != nil {
		return 0, err
	}
	return oracle.Report(ctx)
}
