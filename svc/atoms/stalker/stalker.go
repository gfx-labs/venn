package stalker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"gfx.cafe/gfx/venn/lib/subctx"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/svc/atoms/election"
	"gfx.cafe/gfx/venn/svc/quarks/cluster"
	"gfx.cafe/gfx/venn/svc/services/prom"
)

type Stalker struct {
	ctx       context.Context
	log       *slog.Logger
	headstore headstore.Store
	election  *election.Election

	dt map[string]*delayTracker
}

type Params struct {
	fx.In

	Ctx      context.Context
	Log      *slog.Logger
	Lc       fx.Lifecycle
	Chains   map[string]*config.Chain
	Clusters *cluster.Clusters
	Head     headstore.Store
	Election *election.Election
}

type Result struct {
	fx.Out

	Stalker *Stalker
}

func New(p Params) (r Result, err error) {
	s := &Stalker{
		ctx:       p.Ctx,
		log:       p.Log,
		headstore: p.Head,
		election:  p.Election,
		dt:        make(map[string]*delayTracker),
	}
	r.Stalker = s
	for _, c := range p.Chains {
		chain := c
		blockTime := time.Duration(chain.BlockTimeSeconds * float64(time.Second))
		s.dt[chain.Name] = newDelayTracker(128, 0, int(blockTime))
	}
	p.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go s.election.RunWithLease(
				s.ctx,
				s.log.With("module", "stalker"),
				func(ctx context.Context) {
					for _, chain := range p.Chains {
						if !chain.ParsedStalk {
							continue
						}
						cluster, ok := p.Clusters.Remotes[chain.Name]
						if !ok {
							s.log.Error("cluster not found for chain. not stalking", "chain", chain.Name)
							continue
						}
						go s.stalk(ctx, chain, cluster)
					}
					<-ctx.Done()
				},
				func(ctx context.Context) {
					<-ctx.Done()
				},
			)
			return nil
		},
	})
	return
}

func (T *Stalker) stalk(ctx context.Context, chain *config.Chain, cluster *callcenter.Cluster) {
	// set the chain context for the requests
	ctx = subctx.WithChain(ctx, chain)
	for {
		waitfor, err := T.tick(ctx, chain, cluster)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			T.log.Error("failed to get block head", "chain", chain.Name, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(waitfor):
			continue
		}
	}
}

func (T *Stalker) tick(ctx context.Context, chain *config.Chain, cluster *callcenter.Cluster) (time.Duration, error) {
	blockTime := max(time.Duration(chain.BlockTimeSeconds*float64(time.Second)), 500*time.Millisecond)
	// ask for the latest block
	var block json.RawMessage
	if err := jrpcutil.Do(ctx, cluster, &block, "eth_getBlockByNumber", []any{"latest", false}); err != nil {
		return blockTime, fmt.Errorf("get latest block: %w", err)
	}
	now := time.Now()

	// returned null for latest block, so probably some node is running behind.
	// wait one blockTime and try again
	if bytes.Equal(block, []byte("null")) {
		return blockTime, nil
	}

	// extract block number and timestamp
	var head struct {
		BlockNumber hexutil.Uint64 `json:"number"`
		Timestamp   hexutil.Uint64 `json:"timestamp"`
	}

	if err := json.Unmarshal(block, &head); err != nil {
		return blockTime, err
	}

	objTime := time.Unix(int64(head.Timestamp), 0)
	nextTime := objTime.Add(blockTime)

	prevHead, err := T.headstore.Get(ctx, chain)
	if err != nil {
		return 0, err
	}
	if prevHead != 0 {
		// sanity check, head more than 1000 blocks old? it's a bad head.
		if head.BlockNumber >= prevHead+1000 {
			return blockTime, fmt.Errorf("head more than 1000 blocks old")
		}
	}

	prev, err := T.headstore.Put(ctx, chain, head.BlockNumber)
	if err != nil {
		// store error, so lets just wait the block time
		return blockTime, err
	}
	dt := T.dt[chain.Name]
	if prev != head.BlockNumber {
		// we can use this as a data point for propogation delay. we accept a propogation delay of up to the blocktime
		propDelay := nextTime.Sub(now)
		if propDelay > 0 {
			// add a datapoint
			dt.Add(int(propDelay))
		} else {
			propDelay = 0
		}
		// we got a new block, so we can just use the max of all these values
		nextWait := max(min(nextTime.Sub(now), blockTime), 500*time.Millisecond, blockTime/2)
		meanPropDelay := time.Duration(dt.Mean())
		// if the mean propogation delay is greater than 250ms, we reduce by 10% in order to try give the node a chance to "recover" from said delay, slightly
		// otherwise, we dont actually use the meanPropDelay with the nextwait, because its so small that lets just try to be better.
		if meanPropDelay-250*time.Millisecond > 0 {
			nextWait = nextWait + meanPropDelay*9/10
		}
		stalkerLabel := prom.StalkerLabel{
			Chain: chain.Name,
		}
		prom.Stalker.BlockPropagationDelay(stalkerLabel).Observe(float64(propDelay))
		prom.Stalker.PropagationDelayMean(stalkerLabel).Set(float64(meanPropDelay.Milliseconds()))
		// update the head block metric
		prom.Stalker.HeadBlock(stalkerLabel).Set(float64(head.BlockNumber))

		T.log.Debug("received new block",
			"chain", chain.Name,
			"got", head.BlockNumber, "prev", prev,
			"expected time", nextTime, "now", now,
			"next wait", nextWait,
			"propagation delay", meanPropDelay,
		)

		return nextWait, err
	}
	// you checked the block too early and didnt get a new block. no problem, just wait it out, but do put an info log so we know its happening
	if nextTime.After(now) {
		T.log.Info("requested the block too early",
			"chain", chain.Name,
			"next", nextTime, "now", now,
		)
		return min(max(nextTime.Sub(now), 500*time.Millisecond), blockTime), nil
	}
	// in the case of continuing propogation delay, we take the max of 500ms and the blocktime/4, to avoid spamming nodes
	nextWait := max(500*time.Millisecond, blockTime/4)
	T.log.Debug("received stale block",
		"chain", chain.Name,
		"got", head.BlockNumber, "prev", prev,
		"expected time", nextTime, "now", now,
		"next wait", nextWait,
	)
	return nextWait, nil
	// otherwise, use the time until the expected time, or 500ms, whichever is greater
}
