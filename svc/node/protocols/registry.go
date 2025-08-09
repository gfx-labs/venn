package protocols

import (
	"context"
	"sync"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
)

// Factory constructs a protocol-specific DoctorProbe from chain config
type Factory func(chain *config.Chain) callcenter.DoctorProbe

var (
	mu           sync.RWMutex
	registry     = make(map[string]Factory)
	headMu       sync.RWMutex
	headRegistry = make(map[string]HeadFetcher)
)

// Register binds a protocol name to a probe factory
func Register(protocol string, factory Factory) {
	mu.Lock()
	registry[protocol] = factory
	mu.Unlock()
}

// GetDoctorProbe returns a probe for the given protocol, or nil if none registered
func GetDoctorProbe(protocol string, chain *config.Chain) callcenter.DoctorProbe {
	mu.RLock()
	factory, ok := registry[protocol]
	mu.RUnlock()
	if !ok {
		return nil
	}
	return factory(chain)
}

// HeadFetcher fetches the latest head height for a non-EVM protocol
type HeadFetcher func(ctx context.Context, remote callcenter.Remote, chain *config.Chain) (uint64, error)

// RegisterHead registers a HeadFetcher for a protocol
func RegisterHead(protocol string, fetch HeadFetcher) {
	headMu.Lock()
	headRegistry[protocol] = fetch
	headMu.Unlock()
}

// GetHeadFetcher retrieves a registered HeadFetcher for protocol, if any
func GetHeadFetcher(protocol string) HeadFetcher {
	headMu.RLock()
	f := headRegistry[protocol]
	headMu.RUnlock()
	return f
}
