package cluster

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"time"

	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/lib/subctx"
	"gfx.cafe/gfx/venn/svc/shared/services/prom"
	"gfx.cafe/open/jrpc/contrib/codecs/websocket"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"gfx.cafe/gfx/venn/svc/node/middlewares/blockLookBack"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/contrib/codecs/http"
	"go.uber.org/fx"
	"golang.org/x/time/rate"

	"strings"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
)

// protocol-specific doctor probes
type evmDoctorProbe struct{}
type solanaDoctorProbe struct{ chain *config.Chain }
type nearDoctorProbe struct{ chain *config.Chain }
type suiDoctorProbe struct{ chain *config.Chain }

func (evmDoctorProbe) Check(ctx context.Context, remote callcenter.Remote, chainId int) (uint64, time.Time, error) {
	// EVM: require blockNumber to succeed
	var head hexutil.Uint64
	if err := jrpcutil.Do(ctx, remote, &head, "eth_blockNumber", []any{}); err != nil {
		return 0, time.Now(), err
	}
	// Optional chainId check: done by Doctor after probe
	return uint64(head), time.Now(), nil
}

func (p solanaDoctorProbe) Check(ctx context.Context, remote callcenter.Remote, _ int) (uint64, time.Time, error) {
	// Liveness via getBlockHeight (or getSlot)
	var latest uint64
	method := "getBlockHeight"
	if p.chain != nil && p.chain.Solana != nil && p.chain.Solana.HeadMethod == "getSlot" {
		method = "getSlot"
	}
	if err := jrpcutil.Do(ctx, remote, &latest, method, []any{}); err != nil {
		return 0, time.Now(), err
	}
	// Optional getHealth: do not fail if unsupported
	var health string
	_ = jrpcutil.Do(ctx, remote, &health, "getHealth", []any{})
	// Identity: if genesis hash configured, validate
	if p.chain != nil && p.chain.Solana != nil && p.chain.Solana.GenesisHash != "" {
		var gh string
		if err := jrpcutil.Do(ctx, remote, &gh, "getGenesisHash", []any{}); err == nil {
			// Some providers may append extra suffix; accept prefix match for safety
			if gh != p.chain.Solana.GenesisHash && !strings.HasPrefix(gh, p.chain.Solana.GenesisHash) {
				return latest, time.Now(), fmt.Errorf("genesis mismatch: expected %s got %s", p.chain.Solana.GenesisHash, gh)
			}
		}
	}
	return latest, time.Now(), nil
}

func (p nearDoctorProbe) Check(ctx context.Context, remote callcenter.Remote, _ int) (uint64, time.Time, error) {
	// NEAR liveness via block with finality
	finality := "final"
	if p.chain != nil && p.chain.Near != nil && p.chain.Near.Finality != "" {
		finality = p.chain.Near.Finality
	}
	// Our JSON-RPC helper returns inner result, so unmarshal directly into the payload shape
	var block struct {
		Header struct {
			Height uint64 `json:"height"`
		} `json:"header"`
		ChainID string `json:"chain_id,omitempty"`
	}
	// NEAR expects named params as an object, not wrapped in an array
	if err := jrpcutil.Do(ctx, remote, &block, "block", map[string]string{"finality": finality}); err != nil {
		return 0, time.Now(), err
	}
	// Identity check (optional)
	if p.chain != nil && p.chain.Near != nil && p.chain.Near.GenesisHash != "" {
		var status struct {
			ChainID     string `json:"chain_id"`
			GenesisHash string `json:"genesis_hash"`
		}
		if err := jrpcutil.Do(ctx, remote, &status, "status", []any{}); err == nil {
			if status.GenesisHash != "" && status.GenesisHash != p.chain.Near.GenesisHash {
				return block.Header.Height, time.Now(), fmt.Errorf("genesis mismatch: expected %s got %s", p.chain.Near.GenesisHash, status.GenesisHash)
			}
		}
	}
	return block.Header.Height, time.Now(), nil
}

func (p suiDoctorProbe) Check(ctx context.Context, remote callcenter.Remote, _ int) (uint64, time.Time, error) {
	// Head via sui_getLatestCheckpointSequenceNumber
	method := "sui_getLatestCheckpointSequenceNumber"
	if p.chain != nil && p.chain.Sui != nil && p.chain.Sui.HeadMethod != "" {
		method = p.chain.Sui.HeadMethod
	}
	var latest string
	if err := jrpcutil.Do(ctx, remote, &latest, method, []any{}); err != nil {
		return 0, time.Now(), err
	}
	// Parse numeric string
	var height uint64
	_, err := fmt.Sscan(latest, &height)
	if err != nil {
		return 0, time.Now(), err
	}
	// Optional identity check
	if p.chain != nil && p.chain.Sui != nil && p.chain.Sui.ChainIdentifier != "" {
		var id string
		if err := jrpcutil.Do(ctx, remote, &id, "sui_getChainIdentifier", []any{}); err == nil {
			if id != p.chain.Sui.ChainIdentifier {
				return height, time.Now(), fmt.Errorf("chain identifier mismatch: expected %s got %s", p.chain.Sui.ChainIdentifier, id)
			}
		}
	}
	return height, time.Now(), nil
}

// RemoteTarget holds all middleware instances for a specific remote
type RemoteTarget struct {
	BaseProxy     *callcenter.Proxier
	InputData     *callcenter.InputData
	Collector     *callcenter.Collector
	Logger        *callcenter.Logger
	Backer        *callcenter.Backer
	Validator     *callcenter.Validator
	Doctor        *callcenter.Doctor
	RateLimiter   *callcenter.Ratelimiter
	Filterer      *callcenter.Filterer
	BlockLookBack *blockLookBack.BlockLookBack
}

type Clusters struct {
	Remotes map[string]*callcenter.Cluster

	// Middleware instances stored by chain name, then by remote name
	middlewares map[string]map[string]*RemoteTarget
}

type Params struct {
	fx.In

	Log       *slog.Logger
	Chains    map[string]*config.Chain
	HeadStore headstore.Store
	Lc        fx.Lifecycle
}

type Result struct {
	fx.Out

	Clusters *Clusters
}

// NewRemoteTarget creates a new RemoteTarget from config
func NewRemoteTarget(cfg *config.Remote, chain *config.Chain, log *slog.Logger, headStore headstore.Store) (*RemoteTarget, *callcenter.Proxier) {
	// Create base proxier
	proxier := callcenter.NewProxier(func(ctx context.Context) (jrpc.Conn, error) {
		c, err := jrpc.DialContext(ctx, string(cfg.Url))
		if err != nil {
			return nil, err
		}
		switch cc := c.(type) {
		case *http.Client:
			for key, value := range cfg.Headers {
				cc.SetHeader(key, value)
			}
		case *websocket.Client:
			for key, value := range cfg.Headers {
				cc.SetHeader(key, value)
			}
		}
		return c, nil
	})

	mw := &RemoteTarget{
		BaseProxy: proxier,
		InputData: &callcenter.InputData{},
		Collector: callcenter.NewCollector(
			chain.Name,
			cfg.Name,
		),
		Logger: callcenter.NewLogger(
			log.With("remote", cfg.Name, "chain", chain.Name),
		),
		Backer: callcenter.NewBacker(
			log.With("remote", cfg.Name, "chain", chain.Name),
			cfg.RateLimitBackoff.Duration,
			cfg.ErrorBackoffMin.Duration,
			cfg.ErrorBackoffMax.Duration,
		),
		Validator: callcenter.NewValidator(
			max(time.Minute, time.Duration(float64(time.Second)*2*chain.BlockTimeSeconds)),
		),
		Doctor: callcenter.NewDoctorWithProbe(
			log.With("remote", cfg.Name, "chain", chain.Name),
			chain.Id,
			chain.Name,
			cfg.Name,
			cfg.HealthCheckIntervalMin.Duration,
			cfg.HealthCheckIntervalMax.Duration,
			func() callcenter.DoctorProbe {
				switch chain.Protocol {
				case "solana":
					return solanaDoctorProbe{chain: chain}
				case "near":
					return nearDoctorProbe{chain: chain}
				case "sui":
					return suiDoctorProbe{chain: chain}
				default:
					return evmDoctorProbe{}
				}
			}(),
		),
	}

	if cfg.RateLimit != nil {
		mw.RateLimiter = callcenter.NewRatelimiter(
			rate.Limit(cfg.RateLimit.EventsPerSecond),
			cfg.RateLimit.Burst,
		)
	} else {
		// default values of 50/100
		mw.RateLimiter = callcenter.NewRatelimiter(
			50,
			100,
		)
	}

	methods := make(map[string]bool)
	for _, filter := range cfg.ParsedFilters {
		maps.Copy(methods, filter.Methods)
	}
	mw.Filterer = callcenter.NewFilterer(methods)

	if cfg.MaxBlockLookBack > 0 {
		mw.BlockLookBack = blockLookBack.New(cfg, headStore)
	}

	return mw, proxier
}

func New(params Params) (r Result, err error) {
	r.Clusters = &Clusters{
		Remotes:     make(map[string]*callcenter.Cluster),
		middlewares: make(map[string]map[string]*RemoteTarget),
	}

	// Start a background goroutine to update chain health metrics periodically
	params.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				ticker := time.NewTicker(5 * time.Second) // Update every 5 seconds
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						r.Clusters.UpdateChainHealthMetrics()
					}
				}
			}()
			return nil
		},
	})

	for _, chain := range params.Chains {
		cluster := callcenter.NewCluster()
		r.Clusters.Remotes[chain.Name] = cluster
		// Initialize the nested map for this chain
		r.Clusters.middlewares[chain.Name] = make(map[string]*RemoteTarget)
		for _, cfg := range chain.Remotes {
			cfg := cfg
			toclose := make([]io.Closer, 0)
			params.Lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					// Create RemoteTarget using constructor
					mw, proxier := NewRemoteTarget(cfg, chain, params.Log, params.HeadStore)
					r.Clusters.middlewares[chain.Name][cfg.Name] = mw
					toclose = append(toclose, proxier)

					// Now build the middleware chain
					var remote jrpc.Handler = proxier

					if mw.InputData != nil {
						remote = mw.InputData.Middleware(remote)
					}

					remote = mw.Collector.Middleware(remote)
					remote = mw.Logger.Middleware(remote)
					remote = mw.Backer.Middleware(remote)
					remote = mw.Validator.Middleware(remote)
					remote = mw.Doctor.Middleware(remote)
					remote = mw.RateLimiter.Middleware(remote)
					remote = mw.Filterer.Middleware(remote)

					if mw.BlockLookBack != nil {
						remote = mw.BlockLookBack.Middleware(remote)
					}

					remoteWithConfig := callcenter.NewRemoteWithConfig(remote, cfg)
					cluster.Add(cfg.Priority, remoteWithConfig)

					return nil
				},
				OnStop: func(_ context.Context) error {
					for _, v := range toclose {
						_ = v.Close()
					}
					return nil
				},
			})
		}
	}
	return
}

func (T *Clusters) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		chain, err := subctx.GetChain(r.Context())
		if err != nil {
			_ = w.Send(nil, err)
			return
		}
		remote, ok := T.Remotes[chain.Name]
		if !ok {
			if next != nil {
				next.ServeRPC(w, r)
			} else {
				w.Send(nil, jsonrpc.NewInvalidRequestError("chain not supported: "+chain.Name))
			}
			return
		}
		remote.ServeRPC(w, r)
	})
}

// GetMiddlewares returns the middleware map for dashboard access
func (T *Clusters) GetMiddlewares() map[string]map[string]*RemoteTarget {
	return T.middlewares
}

// UpdateChainHealthMetrics updates chain-level health metrics by examining all doctors for each chain
func (T *Clusters) UpdateChainHealthMetrics() {
	for chainName, remotes := range T.middlewares {
		healthyCount := 0
		totalCount := len(remotes)
		totalRequests := 0.0
		totalSuccesses := 0.0

		for _, remote := range remotes {
			if remote.Doctor.GetHealthStatus() == callcenter.HealthStatusHealthy {
				healthyCount++
			}

			// Aggregate success rate data
			requests := remote.Collector.GetRequestsPerMinute()
			successRate := remote.Collector.GetSuccessRate()
			totalRequests += requests
			totalSuccesses += (requests * successRate / 100.0)
		}

		chainLabel := prom.ChainHealthLabel{
			Chain: chainName,
		}

		// Update chain health metrics
		prom.ChainHealth.HealthyRemoteCount(chainLabel).Set(float64(healthyCount))
		prom.ChainHealth.TotalRemoteCount(chainLabel).Set(float64(totalCount))

		// Calculate availability percentage
		var availabilityPercent float64
		if totalCount > 0 {
			availabilityPercent = float64(healthyCount) / float64(totalCount) * 100
		}
		prom.ChainHealth.AvailabilityPercent(chainLabel).Set(availabilityPercent)

		// Calculate overall request success rate
		var requestSuccessRate float64
		if totalRequests > 0 {
			requestSuccessRate = (totalSuccesses / totalRequests) * 100
		} else {
			requestSuccessRate = 100.0 // No requests means 100% success rate
		}
		prom.ChainHealth.RequestSuccessRate(chainLabel).Set(requestSuccessRate)
	}
}
