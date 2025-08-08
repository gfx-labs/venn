package dashboard

import (
	"embed"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/singleflight"

	"gfx.cafe/gfx/venn/dashboard/templates"
	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/svc/node/quarks/cluster"
)

//go:embed static/*
var staticFiles embed.FS

type Handler struct {
	chains    map[string]*config.Chain
	clusters  *cluster.Clusters
	headstore headstore.Store
	sf        singleflight.Group
}

func NewHandler(chains map[string]*config.Chain, clusters *cluster.Clusters, headstore headstore.Store) *Handler {
	return &Handler{
		chains:    chains,
		clusters:  clusters,
		headstore: headstore,
	}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/dashboard", func(r chi.Router) {
		// Serve static files
		r.Handle("/static/*", http.StripPrefix("/dashboard/", http.FileServer(http.FS(staticFiles))))

		// Dashboard page
		r.Get("/", h.handleDashboard)

		// HTMX endpoints
		r.Get("/chains", h.handleChainsUpdate)

		// Chain detail page
		r.Get("/{chainName}", h.handleChainDetail)

		// HTMX endpoints for chain detail
		r.Get("/{chainName}/remotes", h.handleRemotesUpdate)
	})
}

// setNoCacheHeaders sets headers to prevent caching of dynamic data
func setNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	chains := h.getChainInfos(r)

	// Render template
	component := templates.Index(chains)
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) handleChainsUpdate(w http.ResponseWriter, r *http.Request) {
	setNoCacheHeaders(w)
	chains := h.getChainInfos(r)

	// Render chain cards only
	for _, chain := range chains {
		component := templates.ChainCard(chain)
		if err := component.Render(r.Context(), w); err != nil {
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
			return
		}
	}
}

func (h *Handler) handleChainDetail(w http.ResponseWriter, r *http.Request) {
	chainName := chi.URLParam(r, "chainName")

	chain, exists := h.chains[chainName]
	if !exists {
		http.Error(w, "Chain not found", http.StatusNotFound)
		return
	}

	headBlock, _ := h.headstore.Get(r.Context(), chain)

	// Collect remote information
	remotes := h.getRemoteInfos(chainName, chain, uint64(headBlock))

	healthyCount := 0
	for _, remote := range remotes {
		if remote.Status == callcenter.HealthStatusHealthy {
			healthyCount++
		}
	}

	data := templates.ChainDetailData{
		ChainInfo: templates.ChainInfo{
			Name:           chain.Name,
			ChainID:        uint64(chain.Id),
			HeadBlock:      uint64(headBlock),
			Status:         h.getChainStatus(chainName),
			RemoteCount:    len(remotes),
			HealthyCount:   healthyCount,
			UnhealthyCount: len(remotes) - healthyCount,
		},
		Remotes: remotes,
	}

	// Render template
	component := templates.ChainDetail(data)
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) handleRemotesUpdate(w http.ResponseWriter, r *http.Request) {
	setNoCacheHeaders(w)
	chainName := chi.URLParam(r, "chainName")

	chain, exists := h.chains[chainName]
	if !exists {
		http.Error(w, "Chain not found", http.StatusNotFound)
		return
	}

	headBlock, _ := h.headstore.Get(r.Context(), chain)
	remotes := h.getRemoteInfos(chainName, chain, uint64(headBlock))

	// Render only the remotes list
	component := templates.RemotesList(remotes)
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) getChainInfos(r *http.Request) []templates.ChainInfo {
	// Use singleflight to prevent duplicate work
	result, _, _ := h.sf.Do("getChainInfos", func() (interface{}, error) {
		chains := make([]templates.ChainInfo, 0, len(h.chains))

		for _, chain := range h.chains {
			headBlock, _ := h.headstore.Get(r.Context(), chain)

			// Count healthy remotes
			remotes := h.getRemoteInfos(chain.Name, chain, uint64(headBlock))
			healthyCount := 0
			for _, remote := range remotes {
				if remote.Status == callcenter.HealthStatusHealthy {
					healthyCount++
				}
			}

			chains = append(chains, templates.ChainInfo{
				Name:           chain.Name,
				ChainID:        uint64(chain.Id),
				HeadBlock:      uint64(headBlock),
				Status:         h.getChainStatus(chain.Name),
				RemoteCount:    len(remotes),
				HealthyCount:   healthyCount,
				UnhealthyCount: len(remotes) - healthyCount,
			})
		}

		// Sort chains alphabetically by name
		sort.Slice(chains, func(i, j int) bool {
			return chains[i].Name < chains[j].Name
		})

		return chains, nil
	})

	if result != nil {
		return result.([]templates.ChainInfo)
	}
	return []templates.ChainInfo{}
}

func (h *Handler) getRemoteInfos(chainName string, chain *config.Chain, headBlock uint64) []templates.RemoteInfo {
	// Use singleflight to prevent duplicate work
	key := "getRemoteInfos:" + chainName
	result, _, _ := h.sf.Do(key, func() (interface{}, error) {
		remotes := []templates.RemoteInfo{}

		if middlewares, ok := h.clusters.GetMiddlewares()[chainName]; ok {
			for remoteName, target := range middlewares {
				// Find the config for this remote
				var remoteConfig *config.Remote
				for _, cfg := range chain.Remotes {
					if cfg.Name == remoteName {
						remoteConfig = cfg
						break
					}
				}

				if remoteConfig == nil {
					continue
				}

				status := callcenter.HealthStatusUnknown
				latestBlock := uint64(0)
				responseTime := ""
				var latencyAvg, latencyMin, latencyMax time.Duration
				var lastError string
				priority := remoteConfig.Priority
				maxBlockLookBack := int64(0)
				var requestsPerMin float64

				// Get latest block/timestamp from validator; if not available, fallback to doctor (non-EVM chains)
				var lastUpdated time.Time
				if target.Validator != nil {
					head, updated := target.Validator.GetHead()
					latestBlock = uint64(head)
					lastUpdated = updated
				}

				// Check if doctor exists and get health status
				if target.Doctor != nil {
					// Get health status from doctor
					if target.Doctor.CanUse() {
						status = callcenter.HealthStatusHealthy
					} else {
						status = callcenter.HealthStatusUnhealthy
					}

					// Get latency stats for health checks
					avg, min, max, _ := target.Doctor.GetLatencyStats()
					latencyAvg = avg
					latencyMin = min
					latencyMax = max

					// Get last error
					lastError = target.Doctor.GetLastError()

					// If validator didn't provide head/updated, try doctor fallback (e.g., Solana)
					if latestBlock == 0 {
						if h := target.Doctor.GetLastHead(); h > 0 {
							latestBlock = h
						}
					}
					if lastUpdated.IsZero() {
						if t := target.Doctor.GetLastChecked(); !t.IsZero() {
							lastUpdated = t
						}
					}
				}

				// Calculate response time as time since last update
				if !lastUpdated.IsZero() {
					responseTime = time.Since(lastUpdated).Round(time.Second).String()
				} else {
					responseTime = "N/A"
				}

				// Check if BlockLookBack is configured
				if target.BlockLookBack != nil {
					maxBlockLookBack = int64(remoteConfig.MaxBlockLookBack)
				}

				// Get requests per minute from collector
				if target.Collector != nil {
					requestsPerMin = target.Collector.GetRequestsPerMinute()
				}

				// Calculate blocks behind
				var blocksBehind int64
				if headBlock > 0 && latestBlock > 0 {
					blocksBehind = int64(headBlock) - int64(latestBlock)
				}

				remotes = append(remotes, templates.RemoteInfo{
					Name:             remoteName,
					Status:           status,
					LatestBlock:      latestBlock,
					BlocksBehind:     blocksBehind,
					ResponseTime:     responseTime,
					LatencyAvg:       latencyAvg,
					LatencyMin:       latencyMin,
					LatencyMax:       latencyMax,
					LastError:        lastError,
					Priority:         priority,
					MaxBlockLookBack: maxBlockLookBack,
					RequestsPerMin:   requestsPerMin,
				})
			}
		}

		// Sort remotes by priority (lower is better), then by name
		sort.Slice(remotes, func(i, j int) bool {
			if remotes[i].Priority != remotes[j].Priority {
				return remotes[i].Priority < remotes[j].Priority
			}
			return remotes[i].Name < remotes[j].Name
		})

		return remotes, nil
	})

	if result != nil {
		return result.([]templates.RemoteInfo)
	}
	return []templates.RemoteInfo{}
}

func (h *Handler) getChainStatus(chainName string) callcenter.HealthStatus {
	if cluster, ok := h.clusters.Remotes[chainName]; ok && cluster != nil {
		// Check if any remote is healthy
		if middlewares, ok := h.clusters.GetMiddlewares()[chainName]; ok {
			for _, target := range middlewares {
				if target.Doctor != nil && target.Doctor.CanUse() {
					return callcenter.HealthStatusHealthy
				}
			}
		}
		return callcenter.HealthStatusUnhealthy
	}
	return callcenter.HealthStatusUnknown
}
