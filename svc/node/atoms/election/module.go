package election

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/election"
	"gfx.cafe/gfx/venn/svc/shared/services/redi"
)

type Cluster struct {
}

type Member struct {
}

type Election struct {
	strategy election.Strategy
}

type Params struct {
	fx.In

	Config *config.Election

	Log   *slog.Logger
	Lc    fx.Lifecycle
	Redis *redi.Redis `optional:"true"`
	Ctx   context.Context
}
type Result struct {
	fx.Out

	Election *Election
}

func New(p Params) (o Result, err error) {
	o.Election = &Election{}

	strategyString := p.Config.Strategy

	if strategyString == "" {
		p.Log.Warn("no election strategy specified. autodetecting")
		if p.Redis != nil {
			strategyString = "redis"
		} else {
			strategyString = "alwaysleader"
		}
	}
	logger := p.Log.With("strategy", strategyString)
	switch strategyString {
	case "redilock", "redsync", "redis":
		o.Election.strategy = election.NewRedilockStrategy(
			p.Redis.Namespace(),
			p.Redis.C(),
			logger,
		)
	case "alwaysleader":
		o.Election.strategy = &election.AlwaysLeader{}
	default:
		return o, fmt.Errorf("invalid election strategy: %s", strategyString)
	}
	logger.Info("running election")
	p.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// starting election
			logger.Info("starting election", "strategy", strategyString)
			return o.Election.strategy.Join(p.Ctx)
		},
	})
	return
}

// RunWithLease will run the leader/follower with the supplied strategy until ctx is cancelled.
// the caller is in charge of making sure their leader/follower functions properly returns when the ctx is closed
// otherwise, it may leak.
func (p *Election) RunWithLease(
	ctx context.Context,
	log *slog.Logger,
	leaderFunc func(ctx context.Context),
	followerFunc func(ctx context.Context),
) {
	for {
		leaseCh := p.strategy.AcquireLease(ctx)
		var lctx context.Context
		select {
		case <-ctx.Done():
			return
		case lctx = <-leaseCh:
		default:
			func() {
				fctx, cn := context.WithCancel(ctx)
				defer cn()

				// if the leader lease was not immediately obtained, that's okay.
				// start the followerFunc with the followerCtx
				go followerFunc(fctx)
				// now try to obtain the leader lease again
				log.Info("waiting for leadership lease")
				select {
				case lctx = <-leaseCh:
				case <-ctx.Done():
				}
			}()
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
		log.Info("obtained leadership")
		go leaderFunc(lctx)
		select {
		// on ctx done, its either the top level ctx being cancelled, or leadership being lost
		case <-lctx.Done():
			// if its a lost leadership context, continue the loop, thereby trying to reobtain the lost leadership
			if errors.Is(context.Cause(lctx), election.ErrLostLeadership) {
				continue
			}
			if errors.Is(lctx.Err(), context.Canceled) {
				return
			}
			// the context was closed for some other reason. we return by default.
			log.Error("leadership election loop ended with unknown error", "err", ctx.Err())
			return
		}
	}
}

func (p *Election) IsLeader() bool {
	return p.strategy.IsLeader()
}

func (p *Election) AcquireLease(ctx context.Context) <-chan context.Context {
	return p.strategy.AcquireLease(ctx)
}
