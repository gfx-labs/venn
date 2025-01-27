package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"gfx.cafe/gfx/venn/lib/util"

	"github.com/alecthomas/hcl/v2"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"go.uber.org/fx"
)

type ConfigResult struct {
	fx.Out

	HTTP      *HTTP
	Redis     *Redis `optional:"true"`
	Ratelimit *RateLimit
	Election  *Election
	Scylla    *Scylla
	Chains    map[string]*Chain
	Remotes   []*Remote
	Metrics   *Metrics `optional:"true"`

	Log *slog.Logger
}

func FileParser(file string) func() (ConfigResult, error) {
	return func() (ConfigResult, error) {

		bts, err := os.ReadFile(file)
		if err != nil {
			return ConfigResult{}, err
		}
		godotenv.Load()

		var cfg *Config
		cfg, err = ParseConfig(bts)
		if err != nil {
			return ConfigResult{}, err
		}

		var remotes []*Remote
		for _, chain := range cfg.Chains {
			remotes = append(remotes, chain.Remotes...)
		}
		level := cfg.Logging.Level
		if ll := os.Getenv("SLOG_LEVEL"); ll != "" {
			switch strings.ToLower(ll) {
			case "debug", "0":
				level = slog.LevelDebug
			}
		}
		var logger *slog.Logger
		logFormat := cfg.Logging.Format
		if ll := os.Getenv("SLOG_FORMAT"); ll != "" {
			logFormat = ll
		}
		switch logFormat {
		case "json":
			logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				AddSource: true,
				Level:     level,
			}))
		case "pretty", "tint":
			fallthrough
		default:
			logger = slog.New(tint.NewHandler(os.Stdout, &tint.Options{
				AddSource: true,
				Level:     level,
			}))
		}
		res := ConfigResult{
			HTTP:      &cfg.HTTP,
			Redis:     cfg.Redis,
			Ratelimit: cfg.Ratelimit,
			Chains:    make(map[string]*Chain, len(cfg.Chains)),
			Remotes:   remotes,
			Scylla:    cfg.Scylla,
			Log:       logger,
			Metrics:   cfg.Metrics,
			Election:  &cfg.Election,
		}
		for _, v := range cfg.Chains {
			if _, ok := res.Chains[v.Name]; ok {
				return ConfigResult{}, fmt.Errorf("chain defined multiple times: %s", v.Name)
			}
			res.Chains[v.Name] = v
		}
		return res, nil
	}
}
func ParseConfigFile(file string) (*Config, error) {
	bts, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return ParseConfig(bts)
}

func ParseConfig(datae ...[]byte) (*Config, error) {
	c := &Config{}
	data := bytes.Join(datae, []byte("\r\n"))

	err := hcl.Unmarshal(data, c)
	if err != nil {
		return nil, err
	}

	if c.Redis != nil {
		c.Redis.Namespace = util.Coa(c.Redis.Namespace, "venn-undefined")
	}

	if c.Ratelimit != nil {
		c.Ratelimit.BucketSize = util.Coa(c.Ratelimit.BucketSize, 200)
		c.Ratelimit.BucketDrip = util.Coa(c.Ratelimit.BucketDrip, 100)
		c.Ratelimit.BucketCycleSeconds = util.Coa(c.Ratelimit.BucketCycleSeconds, 10)
	}

	// add all the filters from the presets block to the remotes
	for _, v := range c.Chains {
		v.ParsedStalk, err = util.CoaFunc(func(v *bool) (bool, error) {
			return *v, nil
		}, v.Stalk, true)
		if err != nil {
			return nil, err
		}

		for _, vv := range v.Remotes {
			vv.Chain = v

			for key, value := range vv.Headers {
				vv.Headers[key] = os.ExpandEnv(value)
			}

			vv.ParsedHealthCheckIntervalMin, err = util.CoaFunc(time.ParseDuration, vv.HealthCheckIntervalMin, time.Minute)
			if err != nil {
				return nil, err
			}

			vv.ParsedHealthCheckIntervalMax, err = util.CoaFunc(time.ParseDuration, vv.HealthCheckIntervalMax, time.Hour)
			if err != nil {
				return nil, err
			}

			vv.ParsedRateLimitBackoff, err = util.CoaFunc(time.ParseDuration, vv.RateLimitBackoff, 5*time.Second)
			if err != nil {
				return nil, err
			}

			vv.ParsedErrorBackoffMin, err = util.CoaFunc(time.ParseDuration, vv.ErrorBackoffMin, 5*time.Second)
			if err != nil {
				return nil, err
			}

			vv.ParsedErrorBackoffMax, err = util.CoaFunc(time.ParseDuration, vv.ErrorBackoffMax, 5*time.Second)
			if err != nil {
				return nil, err
			}

			vv.ParsedFilters = make([]*Filter, 0, len(vv.Filters))
			for _, preset := range vv.Filters {
				for _, f := range c.Filters {
					if preset == f.Name {
						vv.ParsedFilters = append(vv.ParsedFilters, f)
					}
				}
			}
		}
	}
	return c, nil
}
