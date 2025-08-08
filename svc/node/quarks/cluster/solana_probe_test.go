package cluster

import (
	"context"
	"testing"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"

	"gfx.cafe/gfx/venn/lib/config"
)

// mockRemote returns a Remote that responds to Solana methods for tests
func mockRemote(genesis string, useSlot bool) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		switch r.Method {
		case "getBlockHeight":
			_ = w.Send(uint64(123456), nil)
			return
		case "getSlot":
			_ = w.Send(uint64(222222), nil)
			return
		case "getHealth":
			_ = w.Send("ok", nil)
			return
		case "getGenesisHash":
			_ = w.Send(genesis, nil)
			return
		default:
			_ = w.Send(nil, jsonrpc.NewMethodNotFoundError(""))
			return
		}
	})
}

func TestSolanaDoctorProbe_Success(t *testing.T) {
	chain := &config.Chain{
		Name:     "solana",
		Protocol: "solana",
		Solana: &config.SolanaConfig{
			GenesisHash: "expected-genesis",
			HeadMethod:  "getBlockHeight",
		},
	}
	probe := solanaDoctorProbe{chain: chain}
	remote := mockRemote("expected-genesis", false)

	if err := probe.Check(context.Background(), remote, 0); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func TestSolanaDoctorProbe_GenesisMismatch(t *testing.T) {
	chain := &config.Chain{
		Name:     "solana",
		Protocol: "solana",
		Solana: &config.SolanaConfig{
			GenesisHash: "expected-genesis",
			HeadMethod:  "getBlockHeight",
		},
	}
	probe := solanaDoctorProbe{chain: chain}
	remote := mockRemote("wrong-genesis", false)

	if err := probe.Check(context.Background(), remote, 0); err == nil {
		t.Fatalf("expected genesis mismatch error, got nil")
	}
}
