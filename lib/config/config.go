package config

import (
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
)

type NodeConfig struct {
	HTTP
	Logging   Logging     `json:"logging,omitempty"`
	Metrics   *Metrics    `json:"metrics,omitempty"`
	Election  Election    `json:"election,omitempty"`
	Redis     Redis       `json:"redis,omitempty"`
	Ratelimit *AbuseLimit `json:"ratelimit,omitempty"`
	Chains    []*Chain    `json:"chains,omitempty"`
	Filters   []*Filter   `json:"filters,omitempty"`
}

type GatewayConfig struct {
	HTTP
	Logging   Logging    `json:"logging,omitempty"`
	Metrics   *Metrics   `json:"metrics,omitempty"`
	Redis     Redis      `json:"redis,omitempty"`
	Nats      *Nats      `json:"nats,omitempty"`
	Telemetry *Telemetry `json:"telemetry,omitempty"`

	Endpoint *EndpointSpec `json:"endpoint,omitempty"`
	Security *Security     `json:"security,omitempty"`
}

type Telemetry struct {
	Enabled               bool                    `json:"enabled,omitempty"`
	JetstreamStreamConfig *jetstream.StreamConfig `json:"jetstream_stream_config,omitempty"`
}

type Security struct {
	TrustedOrigins []string `json:"trusted_origins,omitempty"`
	// these will override used default origin detection, if exist
	TrustedIpHeaders []string `json:"trusted_ip_headers,omitempty"`

	AllowedOrigins []string `json:"allowed_origins,omitempty"`
}

type EndpointSpec struct {
	Name string `json:"name"`
	// paths to proxy from gateway -> venn path
	Paths  map[string]string `json:"paths"`
	Limits EndpointLimits    `json:"limits,omitempty"`

	Methods []string `json:"methods,omitempty"`

	// url to the venn to proxy to
	VennUrl SafeUrl `json:"venn_url"`
}

type EndpointLimits struct {
	Abuse []AbuseLimit `json:"abuse,omitempty"`
	Usage []UsageLimit `json:"usage,omitempty"`
	//TODO: CustomKeyTemplate string `json:"custom_key_template,omitempty"`
}

type AbuseLimit struct {
	Id     string   `json:"id"`
	Total  int      `json:"total"`
	Window Duration `json:"window"`
}

type UsageLimit struct {
	Id string `json:"id"`
}

// possibly shared objects
type Logging struct {
	Format string     `json:"format,omitempty"`
	Level  slog.Level `json:"log_level,omitempty"`
}

type Election struct {
	Strategy string `json:"strategy,omitempty"`
}

type Metrics struct {
	Disabled bool   `json:"disabled,omitempty"`
	Bind     string `json:"bind,omitempty"`
}

type HTTP struct {
	Bind string `json:"bind"`
}

type Redis struct {
	URI       SafeUrl   `json:"uri,omitempty"`
	Cluster   []SafeUrl `json:"cluster,omitempty"`
	Namespace string    `json:"namespace"`
}

type Nats struct {
	URI SafeUrl `json:"uri"`
}

type Chain struct {
	Name    string   `json:"name"`
	Id      int      `json:"id"`
	Aliases []string `json:"aliases,omitempty"`

	BlockTimeSeconds float64 `json:"block_time_seconds"`

	HeadOracles        []*HeadOracles `json:"head_oracles,omitempty"`
	Remotes            []*Remote      `json:"remotes,omitempty"`
	Stalk              *bool          `json:"stalk,omitempty"`
	ParsedStalk        bool           `json:"-"`
	ForgeBlockReceipts bool           `json:"forge_block_receipts,omitempty"`

	// Protocol indicates which RPC protocol the chain speaks. Defaults to "evm".
	Protocol string        `json:"protocol,omitempty"`
	Solana   *SolanaConfig `json:"solana,omitempty"`
	Near     *NearConfig   `json:"near,omitempty"`
	Sui      *SuiConfig    `json:"sui,omitempty"`
}

// SolanaConfig holds optional Solana-specific settings
type SolanaConfig struct {
	// Expected genesis hash for identity checks
	GenesisHash string `json:"genesis_hash,omitempty"`
	// HeadMethod chooses how we derive head: "getBlockHeight" (default) or "getSlot"
	HeadMethod string `json:"head_method,omitempty"`
}

// NearConfig holds optional NEAR-specific settings
type NearConfig struct {
	// Expected genesis hash for identity checks
	GenesisHash string `json:"genesis_hash,omitempty"`
	// Finality for head queries: "final" (default) or "optimistic"
	Finality string `json:"finality,omitempty"`
}

// SuiConfig holds optional Sui-specific settings
type SuiConfig struct {
	// Expected chain identifier (hex string) from sui_getChainIdentifier
	ChainIdentifier string `json:"chain_identifier,omitempty"`
	// RPC method for head. Default: "sui_getLatestCheckpointSequenceNumber"
	HeadMethod string `json:"head_method,omitempty"`
}

type HeadOracles struct {
	Url     SafeUrl `json:"url"`
	CelExpr string  `json:"expr"`
}

type Filter struct {
	Name    string          ` json:"name"`
	Methods map[string]bool ` json:"methods,omitempty"`
}

type Remote struct {
	Name     string            `json:"name"`
	Url      SafeUrl           `json:"url"`
	Desc     string            `help:"optional description" json:"desc,omitempty"`
	Priority int               `json:"priority,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`

	HealthCheckIntervalMin Duration `json:"health_check_interval_min,omitempty"`
	HealthCheckIntervalMax Duration `json:"health_check_interval_max,omitempty"`

	RateLimitBackoff Duration ` json:"rate_limit_backoff,omitempty"`

	ErrorBackoffMin Duration `json:"error_backoff_min,omitempty"`

	ErrorBackoffMax Duration `json:"error_backoff_max,omitempty"`

	Filters       []string  `json:"filters,omitempty"`
	ParsedFilters []*Filter `json:"-"`

	RateLimit *RemoteRateLimit `json:"ratelimit,omitempty"`

	SendDataAndInput bool `json:"send_data_and_input,omitempty"`
	MaxBlockLookBack int  `json:"max_block_look_back,omitempty"`
}

type RemoteRateLimit struct {
	EventsPerSecond float64 `json:"events_per_second"`
	Burst           int     `json:"burst"`
}
