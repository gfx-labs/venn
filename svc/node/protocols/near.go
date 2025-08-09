package protocols

import (
	"context"
	"fmt"
	"time"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
)

type nearDoctorProbe struct{ chain *config.Chain }

func (p nearDoctorProbe) Check(ctx context.Context, remote callcenter.Remote, _ int) (uint64, time.Time, error) {
	finality := "final"
	if p.chain != nil && p.chain.Near != nil && p.chain.Near.Finality != "" {
		finality = p.chain.Near.Finality
	}
	var block struct {
		Header struct {
			Height uint64 `json:"height"`
		} `json:"header"`
		ChainID string `json:"chain_id,omitempty"`
	}
	if err := jrpcutil.Do(ctx, remote, &block, "block", map[string]string{"finality": finality}); err != nil {
		return 0, time.Now(), err
	}
	if p.chain != nil && p.chain.Near != nil && p.chain.Near.GenesisHash != "" {
		var status struct {
			ChainID     string `json:"chain_id"`
			GenesisHash string `json:"genesis_hash"`
		}
		if err := jrpcutil.Do(ctx, remote, &status, "status", []any{}); err == nil {
			if status.GenesisHash != "" && status.GenesisHash != p.chain.Near.GenesisHash {
				return block.Header.Height, time.Now(), fmt.Errorf("genesis mismatch: expected %s got %s", p.chain.Near.GenesisHash, status.GenesisHash)
			}
		}
	}
	return block.Header.Height, time.Now(), nil
}

func init() {
	Register("near", func(chain *config.Chain) callcenter.DoctorProbe {
		return nearDoctorProbe{chain: chain}
	})
    RegisterHead("near", func(ctx context.Context, remote callcenter.Remote, chain *config.Chain) (uint64, error) {
        finality := "final"
        if chain != nil && chain.Near != nil && chain.Near.Finality != "" {
            finality = chain.Near.Finality
        }
        var block struct {
            Header struct {
                Height uint64 `json:"height"`
            } `json:"header"`
        }
        if err := jrpcutil.Do(ctx, remote, &block, "block", map[string]string{"finality": finality}); err != nil {
            return 0, err
        }
        return block.Header.Height, nil
    })
}
