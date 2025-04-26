package callcenter

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"

	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"gfx.cafe/gfx/venn/lib/jrpcutil"
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
	minInterval time.Duration
	maxInterval time.Duration
	remote      Remote

	log *slog.Logger

	timer *time.Timer

	firstCheck sync.WaitGroup
	health     HealthStatus
	interval   time.Duration
	mu         sync.Mutex
}

func NewDoctor(remote Remote, log *slog.Logger, chainId int, minInterval, maxInterval time.Duration) *Doctor {
	ctx, cn := context.WithCancel(context.Background())

	doctor := &Doctor{
		ctx: ctx,
		cn:  cn,

		chainId:     chainId,
		minInterval: minInterval,
		maxInterval: maxInterval,
		remote:      remote,

		log: log,

		timer: time.NewTimer(minInterval),

		interval: minInterval,
	}
	doctor.firstCheck.Add(1)

	go func() {
		doctor.check()

		doctor.loop()
	}()

	return doctor
}

func (T *Doctor) loop() {
	defer T.timer.Stop()

	T.check()
	T.firstCheck.Done()
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

	var res hexutil.Uint64
	err := jrpcutil.Do(ctx, T.remote, &res, "eth_chainId", []any{})
	T.mu.Lock()
	defer T.mu.Unlock()
	func() {
		switch {
		case err != nil:
			T.log.Error("remote failed health check", "chain id", T.chainId, "error", err)
		case int(res) != T.chainId:
			T.log.Error("remote failed health check", "expected id", T.chainId, "got", int(res))
		default:
			T.health = HealthStatusHealthy
			T.interval = min(T.maxInterval, T.interval*2)
			T.timer.Reset(T.interval)
			return
		}
		T.health = HealthStatusUnhealthy
		T.interval = T.minInterval
		T.timer.Reset(T.interval)
	}()
}

// canUse will return true if the remote is healthy, false if it is unhealthy, and will block until the first check is complete.
func (T *Doctor) canUse() bool {
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
	return T.canUse()
}

func (T *Doctor) ServeRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
	if !T.canUse() {
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

var _ Remote = (*Doctor)(nil)
