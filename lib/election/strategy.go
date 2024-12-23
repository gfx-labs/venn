package election

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrLostLeadership = errors.New("lost leadership")

type Announcement struct {
	Id      uuid.UUID `gtrs:"id"`
	Command string    `gtrs:"command"`
	Data    string    `gtrs:"data"`
}

type Cluster struct {
}

type Member struct {
}

type Strategy interface {
	IsLeader() bool
	// Join will join the election and error if it cannot
	// it will run until ctx is cancelled, and will otherwise keep retrying to rejoin the election if it errors
	Join(ctx context.Context) error

	// AcquireLease returns a channel to a context
	// the channel will send and then close once leadership is acquired
	// the context will close if either the parent context is cancelled, or leadership is lost
	// if leadership is lost, context should error with ErrLostLeadership
	AcquireLease(ctx context.Context) <-chan context.Context
}

type AlwaysLeader struct {
	ctx context.Context
	cn  context.CancelFunc
}

func (a *AlwaysLeader) IsLeader() bool {
	return true
}

// Join will join the election and error if it cannot
// it will run until ctx is cancelled, and will otherwise keep retrying to rejoin the election if it errors
// if join exits, all lease should expire
func (a *AlwaysLeader) Join(ctx context.Context) error {
	a.ctx, a.cn = context.WithCancel(ctx)
	defer a.cn()
	select {
	case <-ctx.Done():
		return nil
	}
}

// AcquireLease returns a channel to a context
// the channel will send and then close once leadership is acquired
// the context will close if either the parent context is cancelled, or leadership is lost
// if leadership is lost, context should error with ErrLostLeadership
func (a *AlwaysLeader) AcquireLease(ctx context.Context) <-chan context.Context {
	sctx, cn := context.WithCancel(ctx)
	go func() {
		select {
		case <-a.ctx.Done():
			cn()
		case <-ctx.Done():
			return
		}
	}()
	out := make(chan context.Context)
	out <- sctx
	close(out)
	return out
}
