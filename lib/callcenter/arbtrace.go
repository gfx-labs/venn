package callcenter

import (
	"github.com/gfx-labs/jrpc"
)

// ArbitrumChainIds are the Nitro chains that serve the parity trace namespace
// under an arbtrace_ prefix instead of trace_.
var ArbitrumChainIds = map[int]bool{
	42161:  true, // arbitrum one
	42170:  true, // arbitrum nova
	421614: true, // arbitrum sepolia
}

// arbTraceAliases maps each parity trace method onto its Nitro spelling.
//
// Only the method name differs. Params and results are the same shapes, so a
// caller written against trace_ needs no knowledge of the rename. Methods
// absent here (trace_rawTransaction, trace_replaceBlockTransactions) have no
// arbtrace_ counterpart and are left alone to fail as unsupported rather than
// be rewritten into something that does not exist.
var arbTraceAliases = map[string]string{
	"trace_block":             "arbtrace_block",
	"trace_call":              "arbtrace_call",
	"trace_callMany":          "arbtrace_callMany",
	"trace_filter":            "arbtrace_filter",
	"trace_get":               "arbtrace_get",
	"trace_replayTransaction": "arbtrace_replayTransaction",
	"trace_transaction":       "arbtrace_transaction",
}

// ArbTrace rewrites parity trace_ calls to the arbtrace_ namespace that
// Arbitrum Nitro serves them under.
//
// This is a naming difference, not a behavioural one: which blocks a node can
// trace is the node's business, so an upstream that gains real trace_ support
// should be opted out with disable_arbtrace rather than special cased here.
type ArbTrace struct{}

func (T *ArbTrace) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		if alias, ok := arbTraceAliases[r.Method]; ok {
			r.Method = alias
		}
		next.ServeRPC(w, r)
	})
}
