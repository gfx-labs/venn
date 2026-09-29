package callcenter

import (
	"testing"

	"github.com/gfx-labs/jrpc"
)

// recordingHandler captures the method as the upstream would receive it.
type recordingHandler struct {
	method string
}

func (h *recordingHandler) ServeRPC(_ jrpc.ResponseWriter, r *jrpc.Request) {
	h.method = r.Method
}

func TestArbTraceRewritesMethod(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"filter", "trace_filter", "arbtrace_filter"},
		{"block", "trace_block", "arbtrace_block"},
		{"transaction", "trace_transaction", "arbtrace_transaction"},
		{"call", "trace_call", "arbtrace_call"},
		{"callMany", "trace_callMany", "arbtrace_callMany"},
		{"get", "trace_get", "arbtrace_get"},
		{"replayTransaction", "trace_replayTransaction", "arbtrace_replayTransaction"},
		// Untouched: no arbtrace_ counterpart exists, so these must reach the
		// upstream unchanged and fail there rather than be rewritten into a
		// method that does not exist.
		{"rawTransaction unmapped", "trace_rawTransaction", "trace_rawTransaction"},
		{"replaceBlockTransactions unmapped", "trace_replaceBlockTransactions", "trace_replaceBlockTransactions"},
		// Unrelated namespaces are never touched.
		{"eth passthrough", "eth_getLogs", "eth_getLogs"},
		{"debug passthrough", "debug_traceBlockByNumber", "debug_traceBlockByNumber"},
		// Guards against double rewriting if a caller already speaks Nitro.
		{"already arbtrace", "arbtrace_filter", "arbtrace_filter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &recordingHandler{}
			arb := &ArbTrace{}
			arb.Middleware(next).ServeRPC(nil, &jrpc.Request{Method: tt.in})

			if next.method != tt.want {
				t.Fatalf("method = %q, want %q", next.method, tt.want)
			}
		})
	}
}

// The alias table must only ever map trace_ onto the matching arbtrace_ name,
// so a typo cannot silently point one method at another.
func TestArbTraceAliasesAreConsistent(t *testing.T) {
	for from, to := range arbTraceAliases {
		if want := "arb" + from; to != want {
			t.Errorf("alias %q maps to %q, want %q", from, to, want)
		}
	}
}

func TestArbitrumChainIds(t *testing.T) {
	for _, id := range []int{42161, 42170, 421614} {
		if !ArbitrumChainIds[id] {
			t.Errorf("chain %d should be treated as Arbitrum", id)
		}
	}
	// Chains that serve the parity namespace natively must not be rewritten.
	for _, id := range []int{1, 8453, 10, 137} {
		if ArbitrumChainIds[id] {
			t.Errorf("chain %d should not be treated as Arbitrum", id)
		}
	}
}
