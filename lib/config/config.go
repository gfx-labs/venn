package config

import (
	"log/slog"
	"time"
)

type NodeConfig struct {
	HTTP
	Logging   Logging    `json:"logging,omitempty"`
	Metrics   *Metrics   `json:"metrics,omitempty"`
	Election  Election   `json:"election,omitempty"`
	Redis     *Redis     `json:"redis,omitempty"`
	Ratelimit *RateLimit `json:"ratelimit,omitempty"`
	Chains    []*Chain   `json:"chains,omitempty"`
	Scylla    *Scylla    `json:"scylla,omitempty"`
	Filters   []*Filter  `json:"filters,omitempty"`
}

type GatewayConfig struct {
	HTTP
	Logging Logging  `json:"logging,omitempty"`
	Metrics *Metrics `json:"metrics,omitempty"`
	Redis   *Redis   `json:"redis,omitempty"`
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

type Scylla struct {
	URI      SafeUrl `json:"uri"`
	Keyspace string  `json:"keyspace"`
	CertFile string  `json:"certfile,omitempty"`

	Hosts []string `json:"hosts,omitempty"`
}

type RateLimit struct {
	BucketSize         int `json:"bucket_size,omitempty"`
	BucketDrip         int `json:"bucket_drip,omitempty"`
	BucketCycleSeconds int `json:"bucket_cycle_seconds,omitempty"`
}

type Chain struct {
	Name string `json:"name"`
	Id   int    `json:"id"`

	BlockTimeSeconds float64 `json:"block_time_seconds"`

	HeadOracles        []*HeadOracles `json:"head_oracles,omitempty"`
	Remotes            []*Remote      `json:"remotes,omitempty"`
	Stalk              *bool          `json:"stalk,omitempty"`
	ParsedStalk        bool           `json:"-"`
	ForgeBlockReceipts bool           `json:"forge_block_receipts,omitempty"`
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

	HealthCheckIntervalMin       string        `json:"health_check_interval_min,omitempty"`
	ParsedHealthCheckIntervalMin time.Duration `json:"-"`
	HealthCheckIntervalMax       string        `json:"health_check_interval_max,omitempty"`
	ParsedHealthCheckIntervalMax time.Duration `json:"-"`

	RateLimitBackoff       string        ` json:"rate_limit_backoff,omitempty"`
	ParsedRateLimitBackoff time.Duration `json:"-"`

	ErrorBackoffMin       string        `json:"error_backoff_min,omitempty"`
	ParsedErrorBackoffMin time.Duration `json:"-"`

	ErrorBackoffMax       string        `json:"error_backoff_max,omitempty"`
	ParsedErrorBackoffMax time.Duration `json:"-"`

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
