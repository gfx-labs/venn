package config

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"os"
	"time"

	"gfx.cafe/gfx/venn/internal/hackdonotuse"
)

type EnvExpandable string

func (T *EnvExpandable) MarshalText() ([]byte, error) {
	if T == nil {
		return []byte("<nil>"), nil
	}
	return []byte(*T), nil
}

func (T *EnvExpandable) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(*T))
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
	return []byte(urls.Scheme + "://" + urls.Host), nil
}

func (u *SafeUrl) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(*u))
}

func (u *SafeUrl) UnmarshalJSON(bts []byte) error {
	if hackdonotuse.MakeUrlsUnsafe {
		var s string
		if err := json.Unmarshal(bts, &s); err != nil {
			return err
		}
		*u = SafeUrl(s)
		return nil
	}
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
	Logging   Logging    `hcl:"logging,optional,block" json:"logging,omitempty"`
	Metrics   *Metrics   `hcl:"metrics,optional,block" json:"metrics,omitempty"`
	Election  Election   `hcl:"election,optional,block" json:"election,omitempty"`
	Redis     *Redis     `hcl:"redis,block" json:"redis,omitempty"`
	Ratelimit *RateLimit `hcl:"ratelimit,optional,block" json:"ratelimit,omitempty"`
	Chains    []*Chain   `hcl:"chain,block" json:"chains,omitempty"`
	Scylla    *Scylla    `hcl:"scylla,optional,block" json:"scylla,omitempty"`
	Filters   []*Filter  `hcl:"filter,block" json:"filters,omitempty"`
}

type Logging struct {
	Format string     `hcl:"format,optional" json:"format,omitempty"`
	Level  slog.Level `hcl:"log_level,optional" json:"log_level,omitempty"`
}

type Election struct {
	Strategy string `hcl:"enabled,optional" json:"strategy,omitempty"`
}

type Metrics struct {
	Disabled bool   `hcl:"disabled,optional" json:"disabled,omitempty"`
	Bind     string `hcl:"bind,optional" json:"bind,omitempty"`
}

type HTTP struct {
	Bind string `hcl:"bind" json:"bind"`
}

type Redis struct {
	URI       SafeUrl `hcl:"uri,optional" json:"uri,omitempty"`
	Namespace string  `hcl:"namespace" json:"namespace"`
}

type Scylla struct {
	URI      SafeUrl `hcl:"uri" json:"uri"`
	Keyspace string  `hcl:"keyspace" json:"keyspace"`
	CertFile string  `hcl:"certfile,optional" json:"certfile,omitempty"`

	Hosts []string `hcl:"hosts,optional" json:"hosts,omitempty"`
}

type RateLimit struct {
	BucketSize         int `hcl:"bucket_size,optional" json:"bucket_size,omitempty"`
	BucketDrip         int `hcl:"bucket_drip,optional" json:"bucket_drip,omitempty"`
	BucketCycleSeconds int `hcl:"bucket_cycle_seconds,optional" json:"bucket_cycle_seconds,omitempty"`
}

type Chain struct {
	Name string `hcl:"name,label" json:"name"`
	Id   int    `hcl:"id" json:"id"`

	BlockTimeSeconds float64 `hcl:"block_time_seconds" json:"block_time_seconds"`

	HeadOracles        []*HeadOracles `hcl:"oracles,block" json:"head_oracles,omitempty"`
	Remotes            []*Remote      `hcl:"remote,block" json:"remotes,omitempty"`
	Stalk              *bool          `hcl:"stalk,optional" json:"stalk,omitempty"`
	ParsedStalk        bool           `hcl:"-" json:"-"`
	ForgeBlockReceipts bool           `hcl:"forge_block_receipts,optional" json:"forge_block_receipts,omitempty"`
}

type HeadOracles struct {
	Url     SafeUrl `hcl:"url" json:"url"`
	CelExpr string  `hcl:"expr" json:"expr"`
}

type Filter struct {
	Name    string          `hcl:"name,label" json:"name"`
	Methods map[string]bool `hcl:"methods,optional" json:"methods,omitempty"`
}

type Remote struct {
	Chain *Chain `hcl:"-"`

	Name     string            `hcl:"name,label" json:"name"`
	Url      SafeUrl           `hcl:"url" json:"url"`
	Desc     string            `hcl:"desc,optional" help:"optional description" json:"desc,omitempty"`
	Priority int               `hcl:"priority,optional" json:"priority,omitempty"`
	Headers  map[string]string `hcl:"headers,optional" json:"headers,omitempty"`

	HealthCheckIntervalMin       string        `hcl:"health_check_interval_min,optional" json:"health_check_interval_min,omitempty"`
	ParsedHealthCheckIntervalMin time.Duration `hcl:"-" json:"-"`
	HealthCheckIntervalMax       string        `hcl:"health_check_interval_max,optional" json:"health_check_interval_max,omitempty"`
	ParsedHealthCheckIntervalMax time.Duration `hcl:"-" json:"-"`

	RateLimitBackoff       string        `hcl:"rate_limit_backoff,optional" json:"rate_limit_backoff,omitempty"`
	ParsedRateLimitBackoff time.Duration `hcl:"-" json:"-"`

	ErrorBackoffMin       string        `hcl:"error_backoff_min,optional" json:"error_backoff_min,omitempty"`
	ParsedErrorBackoffMin time.Duration `hcl:"-" json:"-"`

	ErrorBackoffMax       string        `hcl:"error_backoff_max,optional" json:"error_backoff_max,omitempty"`
	ParsedErrorBackoffMax time.Duration `hcl:"-" json:"-"`

	Filters       []string  `hcl:"filters,optional" json:"filters,omitempty"`
	ParsedFilters []*Filter `hcl:"-" json:"-"`

	RateLimit *RemoteRateLimit `hcl:"ratelimit,block,optional" json:"ratelimit,omitempty"`

	SendDataAndInput bool `hcl:"send_data_and_input,optional" json:"send_data_and_input,omitempty"`
	MaxBlockLookBack int  `hcl:"max_block_look_back,optional" json:"max_block_look_back,omitempty"`
}

type RemoteRateLimit struct {
	EventsPerSecond float64 `hcl:"events_per_second" json:"events_per_second"`
	Burst           int     `hcl:"burst" json:"burst"`
}
