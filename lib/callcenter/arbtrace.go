package callcenter

import (
	"encoding/json"

	"github.com/gfx-labs/jrpc"

	"github.com/gfx-labs/venn/lib/ethtypes"
)

// ArbitrumOneChainId is the only Arbitrum chain with pre-Nitro history. Nova
// and Sepolia launched on Nitro, so they have no blocks arbtrace_ can serve.
const ArbitrumOneChainId = 42161

// NitroGenesisBlock is the first Arbitrum One block produced by Nitro.
//
// Blocks below it come from the classic chain, which nodes trace through the
// arbtrace_ namespace. From this block on, arbtrace_ fails with "method handler
// crashed", so trace_ calls for Nitro blocks are passed through unchanged.
const NitroGenesisBlock = 22207817

// arbTraceAliases maps each block-addressable parity trace method onto its
// classic spelling. Methods addressed by transaction hash are absent: their
// block is unknown without a lookup, so they are never rewritten.
var arbTraceAliases = map[string]string{
	"trace_block":    "arbtrace_block",
	"trace_call":     "arbtrace_call",
	"trace_callMany": "arbtrace_callMany",
	"trace_filter":   "arbtrace_filter",
}

// ArbTrace rewrites parity trace_ calls to arbtrace_ when they target only
// pre-Nitro Arbitrum One blocks. Params and results keep the same shapes.
type ArbTrace struct{}

func (T *ArbTrace) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		if alias, ok := arbTraceAliases[r.Method]; ok && preNitro(r.Method, r.Params) {
			r.Method = alias
		}
		next.ServeRPC(w, r)
	})
}

// preNitro reports whether every block the call touches is below
// NitroGenesisBlock. Tags such as latest resolve to Nitro blocks, and anything
// unparseable is treated as Nitro so the request passes through untouched.
func preNitro(method string, raw json.RawMessage) bool {
	var params []json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil || len(params) == 0 {
		return false
	}
	switch method {
	case "trace_filter":
		var filter struct {
			ToBlock *ethtypes.BlockNumber `json:"toBlock"`
		}
		if err := json.Unmarshal(params[0], &filter); err != nil || filter.ToBlock == nil {
			// A missing toBlock defaults to latest.
			return false
		}
		return classic(*filter.ToBlock)
	case "trace_block":
		return blockParam(params[0])
	case "trace_call":
		// trace_call(call, traceTypes, block); block defaults to latest.
		return len(params) >= 3 && blockParam(params[2])
	case "trace_callMany":
		// trace_callMany(calls, block); block defaults to latest.
		return len(params) >= 2 && blockParam(params[1])
	}
	return false
}

func blockParam(raw json.RawMessage) bool {
	var bn ethtypes.BlockNumber
	if err := json.Unmarshal(raw, &bn); err != nil {
		return false
	}
	return classic(bn)
}

// classic reports whether bn is a concrete block below Nitro genesis. Block 0
// is the earliest tag, which is classic history.
func classic(bn ethtypes.BlockNumber) bool {
	return bn >= ethtypes.EarliestBlockNumber && bn < NitroGenesisBlock
}
