package callcenter

import (
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/open/jrpc"
)

// RemoteWithConfig holds a handler along with its configuration
type RemoteWithConfig struct {
	Handler jrpc.Handler
	Config  *config.Remote
}

// NewRemoteWithConfig creates a new RemoteWithConfig
func NewRemoteWithConfig(handler jrpc.Handler, cfg *config.Remote) *RemoteWithConfig {
	return &RemoteWithConfig{
		Handler: handler,
		Config:  cfg,
	}
}