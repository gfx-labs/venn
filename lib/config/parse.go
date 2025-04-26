package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"gfx.cafe/gfx/venn/lib/util"
	"sigs.k8s.io/yaml"

	"github.com/lmittmann/tint"
	"go.uber.org/fx"
)

type NodeConfigResult struct {
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

func FileParser(file string) func() (NodeConfigResult, error) {
	return func() (NodeConfigResult, error) {
		bts, err := os.ReadFile(file)
		if err != nil {
			return NodeConfigResult{}, err
		}

		var cfg *NodeConfig
		cfg, err = ParseConfig(file, bts)
		if err != nil {
			return NodeConfigResult{}, err
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
		logger.Info("config loaded", "file", file)
		res := NodeConfigResult{
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
				return NodeConfigResult{}, fmt.Errorf("chain defined multiple times: %s", v.Name)
			}
			res.Chains[v.Name] = v
		}
		return res, nil
	}
}
func ParseConfig(file string, data []byte) (*NodeConfig, error) {
	c := &NodeConfig{}

	err := yaml.Unmarshal(data, c)
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
		var err error
		v.ParsedStalk, err = util.CoaFunc(func(v *bool) (bool, error) {
			return *v, nil
		}, v.Stalk, true)
		if err != nil {
			return nil, err
		}

		for _, vv := range v.Remotes {
			vv.Chain = v

			if v.Name == "health" {
				return nil, fmt.Errorf(`chain name cannot be "%s"`, v.Name)
			}

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
