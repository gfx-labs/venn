package protocols

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
)

type solanaDoctorProbe struct{ chain *config.Chain }

func (p solanaDoctorProbe) Check(ctx context.Context, remote callcenter.Remote, _ int) (uint64, time.Time, error) {
	var latest uint64
	method := "getBlockHeight"
	if p.chain != nil && p.chain.Solana != nil && p.chain.Solana.HeadMethod == "getSlot" {
		method = "getSlot"
	}
	if err := jrpcutil.Do(ctx, remote, &latest, method, []any{}); err != nil {
		return 0, time.Now(), err
	}
	var health string
	_ = jrpcutil.Do(ctx, remote, &health, "getHealth", []any{})
	if p.chain != nil && p.chain.Solana != nil && p.chain.Solana.GenesisHash != "" {
		var gh string
		if err := jrpcutil.Do(ctx, remote, &gh, "getGenesisHash", []any{}); err == nil {
			if gh != p.chain.Solana.GenesisHash && !strings.HasPrefix(gh, p.chain.Solana.GenesisHash) {
				return latest, time.Now(), fmt.Errorf("genesis mismatch: expected %s got %s", p.chain.Solana.GenesisHash, gh)
			}
		}
	}
	return latest, time.Now(), nil
}

func init() {
	Register("solana", func(chain *config.Chain) callcenter.DoctorProbe {
		return solanaDoctorProbe{chain: chain}
	})
    RegisterHead("solana", func(ctx context.Context, remote callcenter.Remote, chain *config.Chain) (uint64, error) {
        var head uint64
        method := "getBlockHeight"
        if chain != nil && chain.Solana != nil && chain.Solana.HeadMethod == "getSlot" {
            method = "getSlot"
        }
        if err := jrpcutil.Do(ctx, remote, &head, method, []any{}); err != nil {
            return 0, err
        }
        return head, nil
    })
}
