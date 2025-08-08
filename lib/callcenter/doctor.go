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

	probe DoctorProbe

	lastHead    uint64
	lastChecked time.Time
}

// DoctorProbe abstracts protocol-specific health checks
type DoctorProbe interface {
	// Check returns latest observed head (if applicable), the check timestamp, and error if unhealthy
	Check(ctx context.Context, remote Remote, chainId int) (latest uint64, checkedAt time.Time, err error)
}

// evmDefaultProbe keeps backward-compatible behavior when no probe is supplied
type evmDefaultProbe struct{}

func (evmDefaultProbe) Check(ctx context.Context, remote Remote, _ int) (uint64, time.Time, error) {
	var head hexutil.Uint64
	err := jrpcutil.Do(ctx, remote, &head, "eth_blockNumber", []any{})
	return uint64(head), time.Now(), err
}

func NewDoctorWithProbe(log *slog.Logger, chainId int, chainName, remoteName string, minInterval, maxInterval time.Duration, probe DoctorProbe) *Doctor {
	if probe == nil {
		probe = evmDefaultProbe{}
	}
	return &Doctor{
		chainId:       chainId,
		chainName:     chainName,
		remoteName:    remoteName,
		minInterval:   minInterval,
		maxInterval:   maxInterval,
		log:           log,
		interval:      minInterval,
		latencyWindow: rolling.NewTimePolicy(rolling.NewWindow(180), 5*time.Second),
		probe:         probe,
	}
}

// NewDoctor preserves the original constructor signature and defaults to EVM behavior
func NewDoctor(log *slog.Logger, chainId int, chainName, remoteName string, minInterval, maxInterval time.Duration) *Doctor {
	return NewDoctorWithProbe(log, chainId, chainName, remoteName, minInterval, maxInterval, nil)
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

	// Delegate primary health logic to the protocol-specific probe
	latest, checkedAt, err := T.probe.Check(ctx, T.remote, T.chainId)
	if err != nil {
		T.mu.Lock()
		T.log.Error("remote failed health check", "error", err)
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

	// Record health check latency and store last head/check
	checkLatency := time.Since(start)

	// Determine if we should perform EVM chainId verification
	_, isEvmProbe := T.probe.(evmDefaultProbe)
	if !isEvmProbe {
		// Non-EVM: mark healthy immediately
		T.mu.Lock()
		T.latencyWindow.Append(float64(checkLatency.Nanoseconds()))
		prom.RemoteHealth.CheckLatency(healthLabel).Observe(float64(checkLatency.Milliseconds()))
		if latest > 0 {
			T.lastHead = latest
		}
		if !checkedAt.IsZero() {
			T.lastChecked = checkedAt
		}
		T.health = HealthStatusHealthy
		T.lastError = ""
		T.interval = min(T.maxInterval, T.interval*2)
		T.timer.Reset(T.interval)
		prom.RemoteHealth.Status(healthLabel).Set(1)
		prom.RemoteHealth.LastSuccessTimestamp(healthLabel).Set(float64(time.Now().Unix()))
		T.mu.Unlock()
		return
	}

	// EVM: verify chain id matches
	var chainId hexutil.Uint64
	chainIdErr := jrpcutil.Do(ctx, T.remote, &chainId, "eth_chainId", []any{})

	T.mu.Lock()
	defer T.mu.Unlock()
	T.latencyWindow.Append(float64(checkLatency.Nanoseconds()))
	prom.RemoteHealth.CheckLatency(healthLabel).Observe(float64(checkLatency.Milliseconds()))
	if latest > 0 {
		T.lastHead = latest
	}
	if !checkedAt.IsZero() {
		T.lastChecked = checkedAt
	}

	switch {
	case chainIdErr != nil:
		T.log.Error("remote failed health check", "method", "eth_chainId", "error", chainIdErr)
		T.lastError = chainIdErr.Error()
		T.health = HealthStatusUnhealthy
		T.interval = T.minInterval
		T.timer.Reset(T.interval)
		prom.RemoteHealth.CheckFailures(healthLabel).Inc()
		prom.RemoteHealth.Status(healthLabel).Set(0)
	case int(chainId) != T.chainId:
		errMsg := fmt.Sprintf("chain ID mismatch: expected %d, got %d", T.chainId, int(chainId))
		T.log.Error("remote failed health check", "expected id", T.chainId, "got", int(chainId))
		T.lastError = errMsg
		T.health = HealthStatusUnhealthy
		T.interval = T.minInterval
		T.timer.Reset(T.interval)
		prom.RemoteHealth.CheckFailures(healthLabel).Inc()
		prom.RemoteHealth.Status(healthLabel).Set(0)
	default:
		T.health = HealthStatusHealthy
		T.lastError = ""
		T.interval = min(T.maxInterval, T.interval*2)
		T.timer.Reset(T.interval)
		prom.RemoteHealth.Status(healthLabel).Set(1)
		prom.RemoteHealth.LastSuccessTimestamp(healthLabel).Set(float64(time.Now().Unix()))
	}
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
	// For non-EVM probes, do not gate requests on health; pass-through
	if _, isEvmProbe := T.probe.(evmDefaultProbe); isEvmProbe {
		if !T.CanUse() {
			_ = w.Send(nil, ErrUnhealthy)
			return
		}
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

// GetLastHead returns the last observed head during health checks (0 if unknown)
func (T *Doctor) GetLastHead() uint64 {
	T.mu.Lock()
	defer T.mu.Unlock()
	return T.lastHead
}

// GetLastChecked returns the last time a health check was executed
func (T *Doctor) GetLastChecked() time.Time {
	T.mu.Lock()
	defer T.mu.Unlock()
	return T.lastChecked
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
