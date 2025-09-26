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
	Redis     *Redis
	Ratelimit *AbuseLimit
	Election  *Election
	Chains    map[string]*Chain
	Remotes   []*Remote
	Metrics   *Metrics `optional:"true"`

	Log *slog.Logger
}

func NodeFileParser(file string) func() (NodeConfigResult, error) {
	return func() (NodeConfigResult, error) {
		bts, err := os.ReadFile(file)
		if err != nil {
			return NodeConfigResult{}, err
		}

		var cfg *NodeConfig
		cfg, err = ParseNodeConfig(file, bts)
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
		// Auto-detect container environment if no format specified
		if logFormat == "" {
			if isContainerEnvironment() {
				logFormat = "json"
			} else {
				logFormat = "tint"
			}
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
			Redis:     &cfg.Redis,
			Ratelimit: cfg.Ratelimit,
			Chains:    make(map[string]*Chain, len(cfg.Chains)),
			Remotes:   remotes,
			Log:       logger,
			Metrics:   cfg.Metrics,
			Election:  &cfg.Election,
		}
		endpoints := make(map[string]struct{})
		for _, v := range cfg.Chains {
			if _, ok := res.Chains[v.Name]; ok {
				return NodeConfigResult{}, fmt.Errorf("chain name conflict: %s", v.Name)
			}
			endpoints[v.Name] = struct{}{}
			for _, vv := range v.Aliases {
				if _, ok := endpoints[vv]; ok {
					return NodeConfigResult{}, fmt.Errorf("chain alias conflict: %s", vv)
				}
				endpoints[vv] = struct{}{}
			}
			res.Chains[v.Name] = v
		}
		return res, nil
	}
}
func ParseNodeConfig(file string, data []byte) (*NodeConfig, error) {
	c := &NodeConfig{}

	err := yaml.Unmarshal(data, c)
	if err != nil {
		return nil, err
	}

	c.Redis.Namespace = util.Coa(c.Redis.Namespace, "venn-undefined")
	c.Redis.URI = util.Coa(c.Redis.URI, "embedded")

	if c.Ratelimit != nil {
		c.Ratelimit.Total = util.Coa(c.Ratelimit.Total, 2000)
		c.Ratelimit.Window = util.Coa(c.Ratelimit.Window, Duration{time.Second * 10})
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
			if v.Name == "health" {
				return nil, fmt.Errorf(`chain name cannot be "%s"`, v.Name)
			}

			for key, value := range vv.Headers {
				vv.Headers[key] = os.ExpandEnv(value)
			}

			if vv.HealthCheckIntervalMin.Duration == 0 {
				vv.HealthCheckIntervalMin = Duration{time.Minute}
			}
			if vv.HealthCheckIntervalMax.Duration == 0 {
				vv.HealthCheckIntervalMax = Duration{time.Hour}
			}
			if vv.RateLimitBackoff.Duration == 0 {
				vv.RateLimitBackoff = Duration{5 * time.Second}
			}
			if vv.ErrorBackoffMin.Duration == 0 {
				vv.ErrorBackoffMin = Duration{5 * time.Second}
			}
			if vv.ErrorBackoffMax.Duration == 0 {
				vv.ErrorBackoffMax = Duration{5 * time.Second}
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
