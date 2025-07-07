package dashboard

import (
	"embed"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-chi/chi/v5"

	"gfx.cafe/gfx/venn/dashboard/templates"
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
		
		// Chain detail page
		r.Get("/{chainName}", h.handleChainDetail)
		
		// API endpoints
		r.Get("/api/chain/{chainName}", h.handleChainStatus)
	})
}

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	chains := make([]templates.ChainInfo, 0, len(h.chains))
	
	for _, chain := range h.chains {
		headBlock, _ := h.headstore.Get(r.Context(), chain)
		
		// Check if chain cluster exists and get health status
		isHealthy := false
		if cluster, ok := h.clusters.Remotes[chain.Name]; ok {
			// Check if we can get a response from the cluster
			isHealthy = cluster != nil
		}
		
		chains = append(chains, templates.ChainInfo{
			Name:        chain.Name,
			ChainID:     chain.Id,
			HeadBlock:   uint64(headBlock),
			IsHealthy:   isHealthy,
			LastUpdated: time.Now().Format("15:04:05"),
			BlockTime:   chain.BlockTimeSeconds,
		})
	}
	
	templates.RenderDashboard(w, chains)
}

func (h *Handler) handleChainStatus(w http.ResponseWriter, r *http.Request) {
	chainName := chi.URLParam(r, "chainName")
	
	chain, exists := h.chains[chainName]
	if !exists {
		http.Error(w, "Chain not found", http.StatusNotFound)
		return
	}
	
	headBlock, _ := h.headstore.Get(r.Context(), chain)
	
	// Check health status
	isHealthy := false
	if cluster, ok := h.clusters.Remotes[chain.Name]; ok {
		isHealthy = cluster != nil
	}
	
	// Get latest block info from cluster if available
	if cluster, ok := h.clusters.Remotes[chain.Name]; ok && cluster != nil {
		// Get the latest block number from the doctor if it implements GetLatestBlock
		for chainName, remoteTargets := range h.clusters.GetMiddlewares() {
			if chainName == chain.Name {
				for _, target := range remoteTargets {
					if target.Doctor != nil {
						latestBlock := target.Doctor.GetLatestBlock()
						if latestBlock > 0 {
							headBlock = hexutil.Uint64(latestBlock)
						}
					}
				}
			}
		}
	}
	
	chainInfo := templates.ChainInfo{
		Name:        chain.Name,
		ChainID:     chain.Id,
		HeadBlock:   uint64(headBlock),
		IsHealthy:   isHealthy,
		LastUpdated: time.Now().Format("15:04:05"),
		BlockTime:   chain.BlockTimeSeconds,
	}
	
	// Render the full card with HTMX attributes
	templates.RenderChainCardWithHTMX(w, chainInfo)
}

func (h *Handler) handleChainDetail(w http.ResponseWriter, r *http.Request) {
	chainName := chi.URLParam(r, "chainName")
	
	chain, exists := h.chains[chainName]
	if !exists {
		http.Error(w, "Chain not found", http.StatusNotFound)
		return
	}
	
	headBlock, _ := h.headstore.Get(r.Context(), chain)
	
	// Check overall health status
	isHealthy := false
	if cluster, ok := h.clusters.Remotes[chain.Name]; ok {
		isHealthy = cluster != nil
	}
	
	// Collect remote information
	remotes := []templates.RemoteInfo{}
	activeRemotes := 0
	
	if middlewares, ok := h.clusters.GetMiddlewares()[chain.Name]; ok {
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
			
			remoteHealthy := false
			latestBlock := uint64(0)
			
			// Check if doctor is healthy
			if target.Doctor != nil {
				remoteHealthy = target.Doctor.GetLatestBlock() > 0
				latestBlock = uint64(target.Doctor.GetLatestBlock())
			}
			
			if remoteHealthy {
				activeRemotes++
			}
			
			rateLimit := 50.0 // default
			if remoteConfig.RateLimit != nil {
				rateLimit = remoteConfig.RateLimit.EventsPerSecond
			}
			
			remotes = append(remotes, templates.RemoteInfo{
				Name:        remoteName,
				URL:         string(remoteConfig.Url),
				Priority:    remoteConfig.Priority,
				IsHealthy:   remoteHealthy,
				LatestBlock: latestBlock,
				RateLimit:   rateLimit,
				LastError:   "", // Could be populated from error tracking
			})
		}
	}
	
	data := templates.ChainDetailData{
		Chain: templates.ChainInfo{
			Name:        chain.Name,
			ChainID:     chain.Id,
			HeadBlock:   uint64(headBlock),
			IsHealthy:   isHealthy,
			LastUpdated: time.Now().Format("15:04:05"),
			BlockTime:   chain.BlockTimeSeconds,
		},
		Remotes:       remotes,
		ActiveRemotes: activeRemotes,
		TotalRemotes:  len(remotes),
	}
	
	templates.RenderChainDetail(w, data)
}