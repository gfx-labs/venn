package cluster

import (
	"log/slog"
	"testing"

	"gfx.cafe/open/jrpc"
	"github.com/gfx-labs/venn/lib/config"
	"github.com/gfx-labs/venn/lib/jrpcutil"
)

func TestRemoteWhitelistFromYAML(t *testing.T) {
	cfg, err := config.ParseNodeConfig("test.yml", []byte(`
chains:
- name: ethereum
  id: 1
  remotes:
  - name: logs
    url: https://example.invalid
    filters: [logs-only, legacy]
filters:
- name: logs-only
  whitelist: true
  methods:
    eth_getLogs: true
- name: legacy
  methods:
    eth_call: true
`))
	if err != nil {
		t.Fatal(err)
	}
	chain := cfg.Chains[0]
	target, _ := NewRemoteTarget(chain.Remotes[0], chain, slog.Default(), nil)
	for _, method := range []string{"eth_getLogs", "eth_call", "eth_chainId", "debug_traceCall", "custom_method"} {
		t.Run(method, func(t *testing.T) {
			called := false
			handler := target.Filterer.Middleware(jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) { called = true }))
			var response jrpcutil.Interceptor
			handler.ServeRPC(&response, &jrpc.Request{Method: method})
			if called != (method == "eth_getLogs") {
				t.Fatalf("unexpected forwarding: %v", called)
			}
		})
	}
}

func TestRemoteFilterComposition(t *testing.T) {
	tests := []struct {
		name    string
		filters []*config.Filter
		method  string
		allowed bool
	}{
		{"no filters", nil, "custom_method", true},
		{"legacy last wins", []*config.Filter{{Methods: map[string]bool{"eth_call": false}}, {Methods: map[string]bool{"eth_call": true}}}, "eth_call", true},
		{"empty whitelist", []*config.Filter{{Whitelist: true}}, "eth_getLogs", false},
		{"union", []*config.Filter{{Whitelist: true, Methods: map[string]bool{"eth_getLogs": true}}, {Whitelist: true, Methods: map[string]bool{"eth_call": true}}}, "eth_getLogs", true},
		{"whitelist last wins", []*config.Filter{{Whitelist: true, Methods: map[string]bool{"eth_call": true}}, {Whitelist: true, Methods: map[string]bool{"eth_call": false}}}, "eth_call", false},
		{"legacy deny", []*config.Filter{{Methods: map[string]bool{"eth_getLogs": false}}, {Whitelist: true, Methods: map[string]bool{"eth_getLogs": true}}}, "eth_getLogs", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			handler := newRemoteFilterer(tt.filters).Middleware(jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) { called = true }))
			var response jrpcutil.Interceptor
			handler.ServeRPC(&response, &jrpc.Request{Method: tt.method})
			if called != tt.allowed {
				t.Fatalf("forwarded=%v, want %v", called, tt.allowed)
			}
		})
	}
}
