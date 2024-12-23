package blockLookBack

import (
	"context"
	"encoding/json"
	"fmt"
	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/ethtypes"
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
)

type blockLookBackRemote struct {
	callcenter.Remote

	cfg       *config.Remote
	next      callcenter.Remote
	headStore headstore.Store
}

func New(cfg *config.Remote, headStore headstore.Store, next callcenter.Remote) callcenter.Remote {
	return &blockLookBackRemote{
		cfg:       cfg,
		headStore: headStore,
		next:      next,
	}
}

func (m *blockLookBackRemote) ServeRPC(w jrpc.ResponseWriter, r *jrpc.Request) {
	var err error

	switch r.Method {
	case "eth_getBlockByNumber", "eth_getTransactionByBlockNumberAndIndex":
		err = m.check2Param(r, 0)
	case "eth_getTransactionCount":
		err = m.check2Param(r, 1)
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

	m.next.ServeRPC(w, r)
}

func (m *blockLookBackRemote) Close() error {
	return m.next.Close()
}

func (m *blockLookBackRemote) validateBlockNumber(ctx context.Context, blockNumber ethtypes.BlockNumber) error {
	head, err := m.headStore.Get(ctx)
	if err != nil {
		return err
	}

	if (blockNumber == ethtypes.EarliestBlockNumber) || ((blockNumber > 0) &&
		(uint64(blockNumber) < (uint64(head) - uint64(m.cfg.MaxBlockLookBack)))) {
		err = fmt.Errorf("block number, %d, is too old", blockNumber)
	}

	return err
}

func (m *blockLookBackRemote) check2Param(r *jrpc.Request, index int) error {
	var params []json.RawMessage
	if err := json.Unmarshal(r.Params, &params); err != nil {
		return err
	}
	if len(params) != 2 {
		return jsonrpc.NewInvalidParamsError("expected 2 params")
	}

	var blockNumber ethtypes.BlockNumber
	if err := json.Unmarshal(params[index], &blockNumber); err != nil {
		return err
	}

	return m.validateBlockNumber(r.Context(), blockNumber)
}

func (m *blockLookBackRemote) check1Param(r *jrpc.Request) error {
	var params []ethtypes.BlockNumber
	if err := json.Unmarshal(r.Params, &params); err != nil {
		return err
	}
	if len(params) != 1 {
		return jsonrpc.NewInvalidParamsError("expected 1 parameter")
	}

	return m.validateBlockNumber(r.Context(), params[0])
}

func (m *blockLookBackRemote) checkGetLogs(r *jrpc.Request) error {
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
