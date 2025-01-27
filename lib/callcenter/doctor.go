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

	healthy  bool
	interval time.Duration
	mu       sync.Mutex
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

	go func() {
		doctor.check()

		doctor.loop()
	}()

	return doctor
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

	var res hexutil.Uint64
	err := jrpcutil.Do(ctx, T.remote, &res, "eth_chainId", []any{})

	T.mu.Lock()
	defer T.mu.Unlock()
	func() {
		switch {
		default:
			T.healthy = true
			T.interval = min(T.maxInterval, T.interval*2)
			T.timer.Reset(T.interval)
			return
		case int(res) != T.chainId:
			T.log.Error("remote failed health check", "expected id", T.chainId, "got", int(res))
		case err != nil:
			T.log.Error("remote failed health check", "error", err)
		}
		T.healthy = false
		T.interval = T.minInterval
		T.timer.Reset(T.interval)
	}()
}

func (T *Doctor) canUse() bool {
	T.mu.Lock()
	defer T.mu.Unlock()
	if !T.healthy {
		return false
	}

	T.interval = T.minInterval
	T.timer.Reset(T.interval)
	return true
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
		return T.remote.Close()
	}
}

var _ Remote = (*Doctor)(nil)
