package headreplacer

import (
	"context"
	"encoding/json"
	"fmt"
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

// ReplaceBlockNumberInRequest is a helper function that replaces block number parameters in a request.
// It handles eth_call, eth_getBlockByNumber, and eth_getBlockReceipts methods.
// The blockParamIndex specifies which parameter contains the block number (0-indexed).
// Returns the original request if no replacement is needed, or a new request with replaced params.
func (h *HeadReplacer) ReplaceBlockNumberInRequest(ctx context.Context, r *jrpc.Request, blockParamIndex int) (*jrpc.Request, error) {
	chain, err := subctx.GetChain(ctx)
	if err != nil {
		return nil, err
	}

	if len(r.Params) == 0 {
		return nil, jsonrpc.NewInvalidParamsError("expected parameters")
	}

	// Unmarshal params as array of json.RawMessage to handle any number of params
	var params []json.RawMessage
	if err := json.Unmarshal(r.Params, &params); err != nil {
		return nil, err
	}

	// Check if we have enough parameters
	if blockParamIndex >= len(params) {
		return nil, jsonrpc.NewInvalidParamsError(fmt.Sprintf("need at least %d parameters", blockParamIndex+1))
	}

	// Parse the block number parameter
	var blockNumber ethtypes.BlockNumber
	if err := json.Unmarshal(params[blockParamIndex], &blockNumber); err != nil {
		return nil, err
	}

	// Check if we need to replace the block number
	number, passthrough, err := h.toNumericBlockNumber(ctx, chain, &blockNumber)
	if err != nil {
		return nil, err
	}

	// If no replacement needed, return the original request
	if passthrough {
		return r, nil
	}

	// Create a new params array with the replaced block number
	newParams := make([]any, len(params))
	for i := range len(params) {
		if i == blockParamIndex {
			newParams[i] = number
		} else {
			// Keep the original raw JSON for other params
			newParams[i] = params[i]
		}
	}

	r.Params, err = json.Marshal(newParams)
	if err != nil {
		return nil, err
	}

	return r, nil
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
			newReq, err := h.ReplaceBlockNumberInRequest(r.Context(), r, 0)
			if err != nil {
				_ = w.Send(nil, err)
				return
			}
			next.ServeRPC(w, newReq)
			return
		case "eth_getBlockReceipts":
			newReq, err := h.ReplaceBlockNumberInRequest(r.Context(), r, 0)
			if err != nil {
				_ = w.Send(nil, err)
				return
			}
			next.ServeRPC(w, newReq)
			return
		case "eth_call":
			newReq, err := h.ReplaceBlockNumberInRequest(r.Context(), r, 1)
			if err != nil {
				_ = w.Send(nil, err)
				return
			}
			next.ServeRPC(w, newReq)
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
