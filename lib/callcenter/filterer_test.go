package callcenter

import (
	"errors"
	"testing"

	"gfx.cafe/open/jrpc"
	"github.com/gfx-labs/venn/lib/config"
	"github.com/gfx-labs/venn/lib/jrpcutil"
)

func TestFilterer(t *testing.T) {
	tests := []struct {
		name    string
		filter  *Filterer
		method  string
		allowed bool
	}{
		{"legacy unspecified", NewFilterer(nil), "custom_method", true},
		{"legacy true", NewFilterer(map[string]bool{"eth_call": true}), "eth_call", true},
		{"legacy false", NewFilterer(map[string]bool{"eth_call": false}), "eth_call", false},
		{"whitelist allowed", NewFiltererWithWhitelist(nil, map[string]bool{"eth_getLogs": true}), "eth_getLogs", true},
		{"whitelist unknown", NewFiltererWithWhitelist(nil, map[string]bool{"eth_getLogs": true}), "custom_method", false},
		{"whitelist false", NewFiltererWithWhitelist(nil, map[string]bool{"eth_call": false}), "eth_call", false},
		{"empty whitelist", NewFiltererWithWhitelist(nil, map[string]bool{}), "eth_getLogs", false},
		{"legacy cannot widen", NewFiltererWithWhitelist(map[string]bool{"eth_call": true}, map[string]bool{"eth_getLogs": true}), "eth_call", false},
		{"legacy can deny", NewFiltererWithWhitelist(map[string]bool{"eth_getLogs": false}, map[string]bool{"eth_getLogs": true}), "eth_getLogs", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			handler := tt.filter.Middleware(jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
				called = true
				_ = w.Send("ok", nil)
			}))
			var response jrpcutil.Interceptor
			handler.ServeRPC(&response, &jrpc.Request{Method: tt.method})
			if called != tt.allowed {
				t.Fatalf("upstream called=%v, want %v", called, tt.allowed)
			}
			if tt.allowed && response.Error != nil {
				t.Fatal(response.Error)
			}
			if !tt.allowed && !errors.Is(response.Error, ErrMethodNotAllowed) {
				t.Fatalf("expected method rejection, got %v", response.Error)
			}
		})
	}
}

func TestWhitelistClusterFallback(t *testing.T) {
	for _, method := range []string{"eth_getLogs", "eth_call", "custom_method"} {
		t.Run(method, func(t *testing.T) {
			cluster := NewCluster()
			first, second := 0, 0
			restricted := NewFiltererWithWhitelist(nil, map[string]bool{"eth_getLogs": true}).Middleware(jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
				first++
				_ = w.Send("restricted", nil)
			}))
			fallback := jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
				second++
				_ = w.Send("fallback", nil)
			})
			cluster.Add(1, NewRemoteWithConfig(restricted, &config.Remote{}))
			cluster.Add(2, NewRemoteWithConfig(fallback, &config.Remote{}))
			var response jrpcutil.Interceptor
			cluster.ServeRPC(&response, &jrpc.Request{Method: method})
			if response.Error != nil {
				t.Fatal(response.Error)
			}
			if method == "eth_getLogs" {
				if first != 1 || second != 0 {
					t.Fatalf("calls = %d/%d", first, second)
				}
			} else if first != 0 || second != 1 {
				t.Fatalf("calls = %d/%d", first, second)
			}
		})
	}
}

func TestWhitelistAllRemotesReject(t *testing.T) {
	cluster := NewCluster()
	filter := NewFiltererWithWhitelist(nil, map[string]bool{})
	handler := filter.Middleware(jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		t.Fatal("denied request reached upstream")
	}))
	cluster.Add(1, NewRemoteWithConfig(handler, &config.Remote{}))
	cluster.Add(2, NewRemoteWithConfig(handler, &config.Remote{}))
	var response jrpcutil.Interceptor
	cluster.ServeRPC(&response, &jrpc.Request{Method: "eth_call"})
	if !errors.Is(response.Error, ErrMethodNotAllowed) {
		t.Fatalf("expected final filter rejection, got %v", response.Error)
	}
}
