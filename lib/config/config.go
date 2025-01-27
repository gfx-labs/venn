package config

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"os"
	"time"
)

type EnvExpandable string

func (T *EnvExpandable) MarshalText() ([]byte, error) {
	if T == nil {
		return []byte("<nil>"), nil
	}
	return []byte(*T), nil
}

func (T *EnvExpandable) UnmarshalJSON(bts []byte) error {
	var s string
	if err := json.Unmarshal(bts, &s); err != nil {
		return err
	}
	*T = EnvExpandable(os.ExpandEnv(s))
	return nil
}

type SafeUrl string

func (s *SafeUrl) MarshalText() (text []byte, err error) {
	if s == nil {
		return []byte("<nil>"), nil
	}
	urls, err := url.Parse(string(*s))
	if err != nil {
		return nil, err
	}
	return []byte(urls.Scheme + urls.Host), nil
}

func (u *SafeUrl) UnmarshalJSON(bts []byte) error {
	var s EnvExpandable
	if err := json.Unmarshal(bts, &s); err != nil {
		return err
	}
	urls, err := url.Parse(string(s))
	if err != nil {
		return err
	}
	urlString := SafeUrl(urls.String())
	*u = urlString
	return nil
}

type Config struct {
	HTTP
	Election  Election   `hcl:"election,optional,block"`
	Redis     *Redis     `hcl:"redis,block"`
	Scylla    *Scylla    `hcl:"scylla,optional,block"`
	Ratelimit *RateLimit `hcl:"ratelimit,optional,block"`
	Chains    []*Chain   `hcl:"chain,block"`
	Filters   []*Filter  `hcl:"filter,block"`
	Metrics   *Metrics   `hcl:"metrics,optional,block"`
	Logging   Logging    `hcl:"logging,optional,block"`
}

type Logging struct {
	Format string     `hcl:"format,optional"`
	Level  slog.Level `hcl:"log_level,optional"`
}

type Election struct {
	Strategy string `hcl:"enabled,optional"`
}

type Metrics struct {
	Disabled bool   `hcl:"disabled,optional"`
	Bind     string `hcl:"bind,optional"`
}

type HTTP struct {
	Bind string `hcl:"bind"`
}

type Redis struct {
	URI       SafeUrl `hcl:"uri,optional"`
	Namespace string  `hcl:"namespace"`
}

type Scylla struct {
	URI      SafeUrl `hcl:"uri"`
	Keyspace string  `hcl:"keyspace"`
	CertFile string  `hcl:"certfile,optional"`

	Hosts []string `hcl:"hosts,optional"`
}

type RateLimit struct {
	BucketSize         int `hcl:"bucket_size,optional"`
	BucketDrip         int `hcl:"bucket_drip,optional"`
	BucketCycleSeconds int `hcl:"bucket_cycle_seconds,optional"`
}

type Chain struct {
	Name string `hcl:"name,label"`
	Id   int    `hcl:"id"`

	BlockTimeSeconds float64 `hcl:"block_time_seconds"`

	HeadOracles        []*HeadOracles `hcl:"oracles,block"`
	Remotes            []*Remote      `hcl:"remote,block"`
	Stalk              *bool          `hcl:"stalk,optional"`
	ParsedStalk        bool           `hcl:"-"`
	ForgeBlockReceipts bool           `hcl:"forge_block_receipts,optional"`
}

type HeadOracles struct {
	Url     SafeUrl `hcl:"url"`
	CelExpr string  `hcl:"expr"`
}

type Filter struct {
	Name    string          `hcl:"name,label"`
	Methods map[string]bool `hcl:"methods,optional"`
}

type Remote struct {
	Chain *Chain `hcl:"-"`

	Name     string            `hcl:"name,label"`
	Url      SafeUrl           `hcl:"url"`
	Desc     string            `hcl:"desc,optional" help:"optional description"`
	Priority int               `hcl:"priority,optional"`
	Headers  map[string]string `hcl:"headers,optional"`

	HealthCheckIntervalMin       string        `hcl:"health_check_interval_min,optional"`
	ParsedHealthCheckIntervalMin time.Duration `hcl:"-"`
	HealthCheckIntervalMax       string        `hcl:"health_check_interval_max,optional"`
	ParsedHealthCheckIntervalMax time.Duration `hcl:"-"`

	RateLimitBackoff       string        `hcl:"rate_limit_backoff,optional"`
	ParsedRateLimitBackoff time.Duration `hcl:"-"`

	ErrorBackoffMin       string        `hcl:"error_backoff_min,optional"`
	ParsedErrorBackoffMin time.Duration `hcl:"-"`

	ErrorBackoffMax       string        `hcl:"error_backoff_max,optional"`
	ParsedErrorBackoffMax time.Duration `hcl:"-"`

	Filters       []string  `hcl:"filters,optional"`
	ParsedFilters []*Filter `hcl:"-"`

	RateLimit *RemoteRateLimit `hcl:"ratelimit,block,optional"`

	SendDataAndInput bool `hcl:"send_data_and_input,optional"`
	MaxBlockLookBack int  `hcl:"max_block_look_back,optional"`
}

type RemoteRateLimit struct {
	EventsPerSecond float64 `hcl:"events_per_second"`
	Burst           int     `hcl:"burst,optional"`
}
