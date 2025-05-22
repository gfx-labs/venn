package stalker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"gfx.cafe/gfx/venn/lib/subctx"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/ethtypes"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/svc/atoms/cacher"
	"gfx.cafe/gfx/venn/svc/atoms/election"
	"gfx.cafe/gfx/venn/svc/quarks/cluster"
	"gfx.cafe/gfx/venn/svc/services/prom"
)

type Stalker struct {
	ctx      context.Context
	log      *slog.Logger
	store    headstore.Store
	election *election.Election

	dt map[string]*delayTracker
}

type Params struct {
	fx.In

	Ctx      context.Context
	Log      *slog.Logger
	Lc       fx.Lifecycle
	Chains   map[string]*config.Chain
	Cacher   *cacher.Cacher
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
		ctx:      p.Ctx,
		log:      p.Log,
		store:    p.Head,
		election: p.Election,
		dt:       make(map[string]*delayTracker),
	}
	r.Stalker = s
	remote := p.Cacher.Middleware(p.Clusters.Middleware(nil))
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
						go s.stalk(ctx, chain, remote)
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

func (T *Stalker) stalk(ctx context.Context, chain *config.Chain, remote jrpc.Handler) {
	// set the chain context for the requests
	ctx = subctx.WithChain(ctx, chain)
	for {
		waitfor, err := T.tick(ctx, chain, remote)
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

func (T *Stalker) tick(ctx context.Context, chain *config.Chain, remote jrpc.Handler) (time.Duration, error) {
	blockTime := time.Duration(chain.BlockTimeSeconds * float64(time.Second))
	// ask for the latest block
	var block json.RawMessage
	if err := jrpcutil.Do(ctx, remote, &block, "eth_getBlockByNumber", []any{"latest", false}); err != nil {
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

	prev, err := T.store.Put(ctx, chain, head.BlockNumber)
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

func (T *Stalker) toConcreteBlockNumber(ctx context.Context, chain *config.Chain, blockNumbers ...*ethtypes.BlockNumber) (changed bool, passthrough bool, err error) {
	var head hexutil.Uint64
	var hasHead bool
	for _, blockNumber := range blockNumbers {
		switch *blockNumber {
		case ethtypes.LatestBlockNumber:
			if !hasHead {
				head, err = T.store.Get(ctx, chain)
				if err != nil {
					return
				}
				hasHead = true
			}

			*blockNumber = ethtypes.BlockNumber(head)
			changed = true
		case ethtypes.LatestExecutedBlockNumber, ethtypes.PendingBlockNumber, ethtypes.SafeBlockNumber, ethtypes.FinalizedBlockNumber:
			//		err = jsonrpc.NewInvalidParamsError(`expected "latest" or number`) // TODO(garet)
			return changed, true, nil
		default:
			continue
		}
	}

	return
}

func (T *Stalker) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		chain, err := subctx.GetChain(r.Context())
		if err != nil {
			_ = w.Send(nil, err)
			return
		}
		switch r.Method {
		case "eth_chainId":
			_ = w.Send(hexutil.Uint64(chain.Id), nil)
			return
		case "net_version":
			_ = w.Send(strconv.Itoa(chain.Id), nil)
			return
		case "eth_blockNumber":
			_ = w.Send(T.store.Get(r.Context(), chain))
			return
		case "eth_getBlockByNumber":
			// replace latest for current head
			var request []json.RawMessage
			if err := json.Unmarshal(r.Params, &request); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(request) != 2 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 2 parameters"))
				return
			}

			var blockNumber ethtypes.BlockNumber
			if err := json.Unmarshal(request[0], &blockNumber); err != nil {
				_ = w.Send(nil, err)
				return
			}

			changed, _, err := T.toConcreteBlockNumber(r.Context(), chain, &blockNumber)
			if err != nil {
				_ = w.Send(nil, err)
				return
			}

			if changed {
				r.Params, err = json.Marshal([]any{blockNumber, request[1]})
				if err != nil {
					_ = w.Send(nil, err)
					return
				}
			}

			next.ServeRPC(w, r)
			return
		case "eth_getBlockReceipts":
			// replace latest for current head
			var request []ethtypes.BlockNumber
			if err := json.Unmarshal(r.Params, &request); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(request) != 1 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 1 parameter"))
				return
			}

			changed, _, err := T.toConcreteBlockNumber(r.Context(), chain, &request[0])
			if err != nil {
				_ = w.Send(nil, err)
				return
			}

			if changed {
				r.Params, err = json.Marshal(request)
				if err != nil {
					_ = w.Send(nil, err)
					return
				}
			}

			next.ServeRPC(w, r)
			return
		case "eth_call":
			// replace latest for current head
			var request []json.RawMessage
			if err := json.Unmarshal(r.Params, &request); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(request) < 2 || len(request) > 3 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 2-3 parameters"))
				return
			}

			var blockNumber ethtypes.BlockNumber
			if err := json.Unmarshal(request[1], &blockNumber); err != nil {
				_ = w.Send(nil, err)
				return
			}

			changed, _, err := T.toConcreteBlockNumber(r.Context(), chain, &blockNumber)
			if err != nil {
				_ = w.Send(nil, err)
				return
			}

			if changed {
				newParams := []any{request[0], blockNumber}
				if len(request) == 3 {
					newParams = append(newParams, request[2])
				}
				r.Params, err = json.Marshal(newParams)
				if err != nil {
					_ = w.Send(nil, err)
					return
				}
			}

			next.ServeRPC(w, r)
			return
		case "eth_getLogs":
			// replace latest for current head
			var request []ethtypes.FilterQuery
			if err := json.Unmarshal(r.Params, &request); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(request) != 1 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 1 parameter"))
				return
			}

			if request[0].BlockHash != nil {
				next.ServeRPC(w, r)
				return
			} else {
				var fromBlock = ethtypes.LatestBlockNumber
				if request[0].FromBlock != nil {
					fromBlock = *request[0].FromBlock
				}
				var toBlock = ethtypes.LatestBlockNumber
				if request[0].ToBlock != nil {
					toBlock = *request[0].ToBlock
				}
				changed, _, err := T.toConcreteBlockNumber(r.Context(), chain, &fromBlock, &toBlock)
				if err != nil {
					_ = w.Send(nil, err)
					return
				}

				if changed {
					request[0].FromBlock = &fromBlock
					request[0].ToBlock = &toBlock

					r.Params, err = json.Marshal(request)
					if err != nil {
						_ = w.Send(nil, err)
						return
					}
				}

				next.ServeRPC(w, r)
				return
			}
		default:
			next.ServeRPC(w, r)
			return
		}
	})
}
