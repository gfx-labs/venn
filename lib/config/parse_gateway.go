package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gfx.cafe/gfx/venn/lib/util"
	"sigs.k8s.io/yaml"

	"github.com/lmittmann/tint"
	"go.uber.org/fx"
)

type GatewayConfigResult struct {
	fx.Out

	HTTP    *HTTP
	Redis   Redis
	Metrics *Metrics `optional:"true"`

	Endpoints map[string]*EndpointSpec
	Security  *Security

	Log *slog.Logger
}

func GatewayFileParser(file string) func() (GatewayConfigResult, error) {
	return func() (GatewayConfigResult, error) {
		bts, err := os.ReadFile(file)
		if err != nil {
			return GatewayConfigResult{}, err
		}

		var cfg *GatewayConfig
		cfg, err = ParseGatewayConfig(file, bts)
		if err != nil {
			return GatewayConfigResult{}, err
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

		endpoints := make(map[string]*EndpointSpec)
		for _, v := range cfg.Endpoints {
			if _, ok := endpoints[v.Name]; ok {
				return GatewayConfigResult{}, fmt.Errorf("endpoint name conflict: %s", v.Name)
			}
			endpoints[v.Name] = v
		}
		res := GatewayConfigResult{
			HTTP:    &cfg.HTTP,
			Redis:   cfg.Redis,
			Log:     logger,
			Metrics: cfg.Metrics,

			Security:  cfg.Security,
			Endpoints: endpoints,
		}
		return res, nil
	}
}
func ParseGatewayConfig(file string, data []byte) (*GatewayConfig, error) {
	c := &GatewayConfig{}

	err := yaml.Unmarshal(data, c)
	if err != nil {
		return nil, err
	}

	c.Redis.Namespace = util.Coa(c.Redis.Namespace, "gateway-undefined")
	c.Redis.URI = util.Coa(c.Redis.URI, "embedded")

	if c.Security == nil {
		c.Security = &Security{}
	}
	for _, e := range c.Endpoints {
		for idx, v := range e.Limits.Abuse {
			if v.Id == "" {
				return nil, fmt.Errorf("endpoint %s abuse limit %d has no id", e.Name, idx)
			}
		}
		for idx, v := range e.Limits.Usage {
			if v.Id == "" {
				return nil, fmt.Errorf("endpoint %s usage limit %d has no id", e.Name, idx)
			}
		}
	}

	return c, nil
}
