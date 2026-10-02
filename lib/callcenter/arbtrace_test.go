package callcenter

import (
	"encoding/json"
	"fmt"
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

func hexBlock(n int) string { return fmt.Sprintf(`"0x%x"`, n) }

func TestArbTraceNitroBoundary(t *testing.T) {
	last := NitroGenesisBlock - 1
	first := NitroGenesisBlock

	tests := []struct {
		name   string
		method string
		params string
		want   string
	}{
		{"filter classic range", "trace_filter",
			`[{"fromBlock":"0x1","toBlock":"0x2"}]`, "arbtrace_filter"},
		{"filter ends on last classic block", "trace_filter",
			`[{"fromBlock":"0x1","toBlock":` + hexBlock(last) + `}]`, "arbtrace_filter"},
		{"filter ends on nitro genesis", "trace_filter",
			`[{"fromBlock":"0x1","toBlock":` + hexBlock(first) + `}]`, "trace_filter"},
		{"filter spans boundary", "trace_filter",
			`[{"fromBlock":` + hexBlock(last) + `,"toBlock":` + hexBlock(first+10) + `}]`, "trace_filter"},
		{"filter latest", "trace_filter",
			`[{"fromBlock":"0x1","toBlock":"latest"}]`, "trace_filter"},
		{"filter missing toBlock", "trace_filter",
			`[{"fromBlock":"0x1"}]`, "trace_filter"},
		{"filter retail ranking call", "trace_filter",
			`[{"count":10001,"fromAddress":["0x305E76b899885eD07aB887faBDa9D74963057078"],"fromBlock":"0x1e636948","toBlock":"0x1e636f22"}]`, "trace_filter"},

		{"block classic", "trace_block", `[` + hexBlock(last) + `]`, "arbtrace_block"},
		{"block nitro", "trace_block", `[` + hexBlock(first) + `]`, "trace_block"},
		{"block earliest", "trace_block", `["earliest"]`, "arbtrace_block"},
		{"block latest", "trace_block", `["latest"]`, "trace_block"},

		{"call classic", "trace_call", `[{},["trace"],"0x10"]`, "arbtrace_call"},
		{"call default latest", "trace_call", `[{},["trace"]]`, "trace_call"},
		{"callMany classic", "trace_callMany", `[[],"0x10"]`, "arbtrace_callMany"},
		{"callMany default latest", "trace_callMany", `[[]]`, "trace_callMany"},

		{"hash addressed never rewritten", "trace_transaction", `["0xabc"]`, "trace_transaction"},
		{"malformed params pass through", "trace_filter", `{"not":"an array"}`, "trace_filter"},
		{"eth untouched", "eth_getLogs", `[{"fromBlock":"0x1","toBlock":"0x2"}]`, "eth_getLogs"},
		{"already arbtrace", "arbtrace_filter", `[{"fromBlock":"0x1","toBlock":"0x2"}]`, "arbtrace_filter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &recordingHandler{}
			arb := &ArbTrace{}
			arb.Middleware(next).ServeRPC(nil, &jrpc.Request{Method: tt.method, Params: json.RawMessage(tt.params)})
			if next.method != tt.want {
				t.Fatalf("method = %q, want %q", next.method, tt.want)
			}
		})
	}
}

// The alias table must only ever map trace_ onto the matching arbtrace_ name.
func TestArbTraceAliasesAreConsistent(t *testing.T) {
	for from, to := range arbTraceAliases {
		if want := "arb" + from; to != want {
			t.Errorf("alias %q maps to %q, want %q", from, to, want)
		}
	}
}
