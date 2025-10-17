package blockLookBack

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gfx-labs/venn/lib/callcenter"
	"github.com/gfx-labs/venn/lib/config"
	"github.com/gfx-labs/venn/lib/ethtypes"
	"github.com/gfx-labs/venn/lib/stores/headstore"
	"github.com/gfx-labs/venn/lib/subctx"
	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
)

// BlockLookBack is a middleware that prevents requests for blocks that are too old
type BlockLookBack struct {
	chainCfg  *config.Chain
	remoteCfg *config.Remote
	headStore headstore.Store
}

func New(chainCfg *config.Chain, remoteCfg *config.Remote, headStore headstore.Store) *BlockLookBack {
	return &BlockLookBack{
		chainCfg:  chainCfg,
		remoteCfg: remoteCfg,
		headStore: headStore,
	}
}

// getEffectiveLookBack returns the effective lookback limit
// For chain-level middleware, remoteCfg is nil, so only chain limit is used
func (m *BlockLookBack) getEffectiveLookBack() int {
	chainLimit := m.chainCfg.MaxBlockLookBack

	// For chain-level middleware (remoteCfg is nil), use chain limit
	if m.remoteCfg == nil {
		return chainLimit
	}

	remoteLimit := m.remoteCfg.MaxBlockLookBack

	// If both are 0, no limit
	if chainLimit == 0 && remoteLimit == 0 {
		return 0
	}
	// If only chain is set, use it
	if remoteLimit == 0 {
		return chainLimit
	}
	// If only remote is set, use it
	if chainLimit == 0 {
		return remoteLimit
	}
	// Both are set, use minimum (most restrictive)
	if chainLimit < remoteLimit {
		return chainLimit
	}
	return remoteLimit
}

func (m *BlockLookBack) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		var err error
		switch r.Method {
		case "eth_getBlockByNumber", "eth_getTransactionByBlockNumberAndIndex":
			err = m.check2Param(r, 0)
		case "eth_getTransactionCount", "eth_getBalance", "eth_getCode", "eth_call", "eth_estimateGas", "eth_createAccessList":
			err = m.check2Param(r, 1)
		case "eth_getStorageAt":
			err = m.check3Param(r, 2)
		case "eth_getBlockReceipts", "eth_getBlockTransactionCountByNumber",
			"eth_getUncleCountByBlockNumber", "debug_getRawHeader",
			"debug_getRawBlock":
			err = m.check1Param(r)
		case "eth_getLogs":
			err = m.checkGetLogs(r)
		}

		if err != nil {
			_ = w.Send(nil, err)
			return
		}

		next.ServeRPC(w, r)
	})
}

func (m *BlockLookBack) validateBlockNumber(ctx context.Context, blockNumber ethtypes.BlockNumber) error {
	effectiveLookBack := m.getEffectiveLookBack()

	// If no lookback limit is configured, allow all blocks
	if effectiveLookBack == 0 {
		return nil
	}

	chain, err := subctx.GetChain(ctx)
	if err != nil {
		return err
	}
	head, err := m.headStore.Get(ctx, chain)
	if err != nil {
		return err
	}

	if (blockNumber == ethtypes.EarliestBlockNumber) || ((blockNumber > 0) &&
		(uint64(blockNumber) < (uint64(head) - uint64(effectiveLookBack)))) {
		err = fmt.Errorf("block number, %d, is too old (max lookback: %d blocks from head: %d)", blockNumber, effectiveLookBack, head)
	}

	return err
}

func (m *BlockLookBack) check2Param(r *jrpc.Request, index int) error {
	var params []json.RawMessage
	if err := json.Unmarshal(r.Params, &params); err != nil {
		return err
	}
	if len(params) < 2 {
		return nil // Block parameter is optional for some methods
	}

	var blockNumber ethtypes.BlockNumber
	if err := json.Unmarshal(params[index], &blockNumber); err != nil {
		return err
	}

	return m.validateBlockNumber(r.Context(), blockNumber)
}

func (m *BlockLookBack) check3Param(r *jrpc.Request, index int) error {
	var params []json.RawMessage
	if err := json.Unmarshal(r.Params, &params); err != nil {
		return err
	}
	if len(params) < 3 {
		return nil // Block parameter is optional
	}

	var blockNumber ethtypes.BlockNumber
	if err := json.Unmarshal(params[index], &blockNumber); err != nil {
		return err
	}

	return m.validateBlockNumber(r.Context(), blockNumber)
}

func (m *BlockLookBack) check1Param(r *jrpc.Request) error {
	var params []ethtypes.BlockNumber
	if err := json.Unmarshal(r.Params, &params); err != nil {
		return err
	}
	if len(params) != 1 {
		return jsonrpc.NewInvalidParamsError("expected 1 parameter")
	}

	return m.validateBlockNumber(r.Context(), params[0])
}

func (m *BlockLookBack) checkGetLogs(r *jrpc.Request) error {
	var params []ethtypes.FilterQuery
	if err := json.Unmarshal(r.Params, &params); err != nil {
		return err
	}
	if len(params) != 1 {
		return jsonrpc.NewInvalidParamsError("expected 1 parameter")
	}

	if params[0].FromBlock != nil {
		return m.validateBlockNumber(r.Context(), *params[0].FromBlock)
	}

	return nil
}

var _ callcenter.Middleware = (*BlockLookBack)(nil)
