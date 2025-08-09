package protocols

import (
	"context"
	"fmt"
	"time"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
)

type suiDoctorProbe struct{ chain *config.Chain }

func (p suiDoctorProbe) Check(ctx context.Context, remote callcenter.Remote, _ int) (uint64, time.Time, error) {
	method := "sui_getLatestCheckpointSequenceNumber"
	if p.chain != nil && p.chain.Sui != nil && p.chain.Sui.HeadMethod != "" {
		method = p.chain.Sui.HeadMethod
	}
	var latest string
	if err := jrpcutil.Do(ctx, remote, &latest, method, []any{}); err != nil {
		return 0, time.Now(), err
	}
	var height uint64
	_, err := fmt.Sscan(latest, &height)
	if err != nil {
		return 0, time.Now(), err
	}
	if p.chain != nil && p.chain.Sui != nil && p.chain.Sui.ChainIdentifier != "" {
		var id string
		if err := jrpcutil.Do(ctx, remote, &id, "sui_getChainIdentifier", []any{}); err == nil {
			if id != p.chain.Sui.ChainIdentifier {
				return height, time.Now(), fmt.Errorf("chain identifier mismatch: expected %s got %s", p.chain.Sui.ChainIdentifier, id)
			}
		}
	}
	return height, time.Now(), nil
}

func init() {
	Register("sui", func(chain *config.Chain) callcenter.DoctorProbe {
		return suiDoctorProbe{chain: chain}
	})
	RegisterHead("sui", func(ctx context.Context, remote callcenter.Remote, chain *config.Chain) (uint64, error) {
		method := "sui_getLatestCheckpointSequenceNumber"
		if chain != nil && chain.Sui != nil && chain.Sui.HeadMethod != "" {
			method = chain.Sui.HeadMethod
		}
		var latest string
		if err := jrpcutil.Do(ctx, remote, &latest, method, []any{}); err != nil {
			return 0, err
		}
		var height uint64
		if _, err := fmt.Sscan(latest, &height); err != nil {
			return 0, err
		}
		return height, nil
	})
}
