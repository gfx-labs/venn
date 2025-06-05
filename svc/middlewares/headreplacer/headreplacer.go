package headreplacer

import (
	"context"
	"encoding/json"
	"strconv"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/ethtypes"
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/lib/subctx"
	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/fx"
)

// HeadReplacer is a middleware that replaces "latest" block tags with actual block numbers
type HeadReplacer struct {
	headstore headstore.Store
	chains    map[string]*config.Chain
}

type Params struct {
	fx.In

	Head   headstore.Store
	Chains map[string]*config.Chain
}

type Result struct {
	fx.Out

	HeadReplacer *HeadReplacer
}

func New(p Params) (r Result, err error) {
	r.HeadReplacer = &HeadReplacer{
		headstore: p.Head,
		chains:    p.Chains,
	}
	return
}

func (h *HeadReplacer) toNumericBlockNumber(ctx context.Context, chain *config.Chain, blockNumber *ethtypes.BlockNumber) (value ethtypes.BlockNumber, passthrough bool, err error) {
	switch *blockNumber {
	case ethtypes.LatestBlockNumber:
		head, err := h.headstore.Get(ctx, chain)
		if err != nil {
			return 0, true, err
		}
		if head == 0 {
			return 0, true, nil
		}
		return ethtypes.BlockNumber(head), false, nil
	case ethtypes.LatestExecutedBlockNumber, ethtypes.PendingBlockNumber, ethtypes.SafeBlockNumber, ethtypes.FinalizedBlockNumber:
		return 0, true, nil
	default:
		return 0, true, nil
	}
}

func (h *HeadReplacer) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		chain, err := subctx.GetChain(r.Context())
		if err != nil {
			_ = w.Send(nil, err)
			return
		}
		switch r.Method {
		case "eth_chainId":
			_ = w.Send(hexutil.Uint64(chain.Id), nil)
			return
		case "net_version":
			_ = w.Send(strconv.Itoa(chain.Id), nil)
			return
		case "eth_blockNumber":
			_ = w.Send(h.headstore.Get(r.Context(), chain))
			return
		case "eth_getBlockByNumber":
			// replace latest for current head
			var request []json.RawMessage
			if err := json.Unmarshal(r.Params, &request); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(request) != 2 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 2 parameters"))
				return
			}

			var blockNumber ethtypes.BlockNumber
			if err := json.Unmarshal(request[0], &blockNumber); err != nil {
				_ = w.Send(nil, err)
				return
			}
			number, passthrough, err := h.toNumericBlockNumber(r.Context(), chain, &blockNumber)
			if err != nil {
				_ = w.Send(nil, err)
				return
			}
			if !passthrough {
				r.Params, err = json.Marshal([]any{number, request[1]})
				if err != nil {
					_ = w.Send(nil, err)
					return
				}
			}
			next.ServeRPC(w, r)
			return
		case "eth_getBlockReceipts":
			// replace latest for current head
			var request []ethtypes.BlockNumber
			if err := json.Unmarshal(r.Params, &request); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(request) != 1 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 1 parameter"))
				return
			}

			number, passthrough, err := h.toNumericBlockNumber(r.Context(), chain, &request[0])
			if err != nil {
				_ = w.Send(nil, err)
				return
			}
			if !passthrough {
				r.Params, err = json.Marshal([]any{number})
				if err != nil {
					_ = w.Send(nil, err)
					return
				}
			}

			next.ServeRPC(w, r)
			return
		case "eth_call":
			// replace latest for current head
			var request []json.RawMessage
			if err := json.Unmarshal(r.Params, &request); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(request) < 2 || len(request) > 3 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 2-3 parameters"))
				return
			}

			var blockNumber ethtypes.BlockNumber
			if err := json.Unmarshal(request[1], &blockNumber); err != nil {
				_ = w.Send(nil, err)
				return
			}

			number, passthrough, err := h.toNumericBlockNumber(r.Context(), chain, &blockNumber)
			if err != nil {
				_ = w.Send(nil, err)
				return
			}

			if !passthrough {
				newParams := []any{request[0], number}
				if len(request) == 3 {
					newParams = append(newParams, request[2])
				}
				r.Params, err = json.Marshal(newParams)
				if err != nil {
					_ = w.Send(nil, err)
					return
				}
			}

			next.ServeRPC(w, r)
			return
		case "eth_getLogs":
			// replace latest for current head
			var request []ethtypes.FilterQuery
			if err := json.Unmarshal(r.Params, &request); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(request) != 1 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 1 parameter"))
				return
			}

			if request[0].BlockHash != nil {
				next.ServeRPC(w, r)
				return
			} else {
				var fromBlock = ethtypes.LatestBlockNumber
				if request[0].FromBlock != nil {
					fromBlock = *request[0].FromBlock
				}
				var toBlock = ethtypes.LatestBlockNumber
				if request[0].ToBlock != nil {
					toBlock = *request[0].ToBlock
				}

				numberFromBlock, passthrough1, err := h.toNumericBlockNumber(r.Context(), chain, &fromBlock)
				if err != nil {
					_ = w.Send(nil, err)
					return
				}
				numberToBlock, passthrough2, err := h.toNumericBlockNumber(r.Context(), chain, &toBlock)
				if err != nil {
					_ = w.Send(nil, err)
					return
				}

				if !passthrough1 && !passthrough2 {
					request[0].FromBlock = &numberFromBlock
					request[0].ToBlock = &numberToBlock
					r.Params, err = json.Marshal(request)
					if err != nil {
						_ = w.Send(nil, err)
						return
					}
				}

				next.ServeRPC(w, r)
				return
			}
		default:
			next.ServeRPC(w, r)
			return
		}
	})
}

