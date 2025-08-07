package callcenter

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/svc/shared/services/prom"
	"github.com/asecurityteam/rolling"
)

type HealthStatus int

const (
	HealthStatusUnknown   HealthStatus = 0
	HealthStatusHealthy   HealthStatus = 1
	HealthStatusUnhealthy HealthStatus = -1
)

// Doctor runs periodic health checks on the remote, preventing further requests if the health check fails.
type Doctor struct {
	ctx context.Context
	cn  func()

	chainId     int
	chainName   string
	remoteName  string
	minInterval time.Duration
	maxInterval time.Duration
	remote      Remote

	log *slog.Logger

	timer *time.Timer

	firstCheck    sync.WaitGroup
	health        HealthStatus
	interval      time.Duration
	latencyWindow *rolling.TimePolicy
	lastError     string
	mu            sync.Mutex
}

func NewDoctor(log *slog.Logger, chainId int, chainName, remoteName string, minInterval, maxInterval time.Duration) *Doctor {
	return &Doctor{
		chainId:       chainId,
		chainName:     chainName,
		remoteName:    remoteName,
		minInterval:   minInterval,
		maxInterval:   maxInterval,
		log:           log,
		interval:      minInterval,
		latencyWindow: rolling.NewTimePolicy(rolling.NewWindow(180), 5*time.Second), // 15-minute window: 180 buckets × 5 seconds each
	}
}

func (T *Doctor) Middleware(next jrpc.Handler) jrpc.Handler {
	// Initialize the doctor when middleware is applied
	ctx, cn := context.WithCancel(context.Background())
	T.ctx = ctx
	T.cn = cn
	T.remote = next
	T.timer = time.NewTimer(T.minInterval)
	T.firstCheck.Add(1)

	go func() {
		T.check()
		T.firstCheck.Done()
		T.loop()
	}()

	return T
}

func (T *Doctor) loop() {
	defer T.timer.Stop()

	for {
		select {
		case <-T.ctx.Done():
			return
		case <-T.timer.C:
			T.check()
		}
	}
}

func (T *Doctor) check() {
	ctx, cn := context.WithTimeout(T.ctx, 15*time.Second)
	defer cn()

	// Track health check latency
	start := time.Now()

	// Create label for metrics
	healthLabel := prom.RemoteHealthLabel{
		Chain:  T.chainName,
		Remote: T.remoteName,
	}

	// First check eth_blockNumber to ensure the node is syncing
	var blockNumber hexutil.Uint64
	err := jrpcutil.Do(ctx, T.remote, &blockNumber, "eth_blockNumber", []any{})
	if err != nil {
		T.mu.Lock()
		T.log.Error("remote failed health check", "method", "eth_blockNumber", "error", err)
		T.health = HealthStatusUnhealthy
		T.lastError = err.Error()
		T.interval = T.minInterval
		T.timer.Reset(T.interval)
		T.mu.Unlock()

		// Update metrics for failure
		checkLatency := time.Since(start)
		prom.RemoteHealth.CheckLatency(healthLabel).Observe(float64(checkLatency.Milliseconds()))
		prom.RemoteHealth.CheckFailures(healthLabel).Inc()
		prom.RemoteHealth.Status(healthLabel).Set(-1) // Unhealthy
		return
	}

	// Then verify chain ID
	var chainId hexutil.Uint64
	err = jrpcutil.Do(ctx, T.remote, &chainId, "eth_chainId", []any{})

	// Record health check latency
	checkLatency := time.Since(start)

	T.mu.Lock()
	defer T.mu.Unlock()

	// Always record the health check latency
	T.latencyWindow.Append(float64(checkLatency.Nanoseconds()))
	prom.RemoteHealth.CheckLatency(healthLabel).Observe(float64(checkLatency.Milliseconds()))

	func() {
		switch {
		case err != nil:
			T.log.Error("remote failed health check", "method", "eth_chainId", "error", err)
			T.lastError = err.Error()
		case int(chainId) != T.chainId:
			errMsg := fmt.Sprintf("chain ID mismatch: expected %d, got %d", T.chainId, int(chainId))
			T.log.Error("remote failed health check", "expected id", T.chainId, "got", int(chainId))
			T.lastError = errMsg
		default:
			T.health = HealthStatusHealthy
			T.lastError = ""
			T.interval = min(T.maxInterval, T.interval*2)
			T.timer.Reset(T.interval)

			// Update metrics for success
			prom.RemoteHealth.Status(healthLabel).Set(1) // Healthy
			prom.RemoteHealth.LastSuccessTimestamp(healthLabel).Set(float64(time.Now().Unix()))
			return
		}
		T.health = HealthStatusUnhealthy
		T.interval = T.minInterval
		T.timer.Reset(T.interval)

		// Update metrics for failure
		prom.RemoteHealth.CheckFailures(healthLabel).Inc()
		prom.RemoteHealth.Status(healthLabel).Set(0) // Unhealthy
	}()
}

// CanUse will return true if the remote is healthy, false if it is unhealthy, and will block until the first check is complete.
func (T *Doctor) CanUse() bool {
	T.mu.Lock()
	if T.health == HealthStatusHealthy {
		T.interval = T.minInterval
		T.timer.Reset(T.interval)
		T.mu.Unlock()
		return true
	}
	if T.health == HealthStatusUnhealthy {
		T.mu.Unlock()
		return false
	}
	T.mu.Unlock()
	// wait on the first check to complete.
	T.firstCheck.Wait()
	// at this point, we know we are no longer state unknown, so we can do the check again
	return T.CanUse()
}

func (T *Doctor) ServeRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
	if !T.CanUse() {
		_ = w.Send(nil, ErrUnhealthy)
		return
	}

	T.remote.ServeRPC(w, r)
}

func (T *Doctor) Close() error {
	select {
	case <-T.ctx.Done():
		return net.ErrClosed
	default:
		T.cn()
		return nil
	}
}

// GetLatencyStats returns the latency statistics for health checks
func (T *Doctor) GetLatencyStats() (avg, min, max time.Duration, count int) {
	T.mu.Lock()
	defer T.mu.Unlock()

	// Use Reduce to get stats from the window
	count = int(T.latencyWindow.Reduce(func(w rolling.Window) float64 {
		return rolling.Count(w)
	}))

	if count == 0 {
		return 0, 0, 0, 0
	}

	// Calculate average using Sum/Count
	sum := T.latencyWindow.Reduce(func(w rolling.Window) float64 {
		return rolling.Sum(w)
	})
	avg = time.Duration(sum / float64(count))

	min = time.Duration(T.latencyWindow.Reduce(func(w rolling.Window) float64 {
		return rolling.Min(w)
	}))

	max = time.Duration(T.latencyWindow.Reduce(func(w rolling.Window) float64 {
		return rolling.Max(w)
	}))

	return
}

// GetLastError returns the last error from health checks
func (T *Doctor) GetLastError() string {
	T.mu.Lock()
	defer T.mu.Unlock()
	return T.lastError
}

// GetHealthStatus returns the current health status
func (T *Doctor) GetHealthStatus() HealthStatus {
	T.mu.Lock()
	defer T.mu.Unlock()
	return T.health
}

// GetChainName returns the chain name for this doctor
func (T *Doctor) GetChainName() string {
	return T.chainName
}

// GetRemoteName returns the remote name for this doctor
func (T *Doctor) GetRemoteName() string {
	return T.remoteName
}

var _ Remote = (*Doctor)(nil)
var _ Middleware = (*Doctor)(nil)
