package stalker

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/ethtypes"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/lib/util"
	"gfx.cafe/gfx/venn/svc/atoms/cacher"
	"gfx.cafe/gfx/venn/svc/atoms/election"
	"gfx.cafe/gfx/venn/svc/atoms/vennstore"
	"gfx.cafe/gfx/venn/svc/quarks/cluster"
)

type Stalker struct {
	ctx      context.Context
	log      *slog.Logger
	chain    *config.Chain
	remote   jrpc.Handler
	store    headstore.Store
	election *election.Election
}

type Stalkers = util.Multichain[*Stalker]

type Params struct {
	fx.In

	Ctx      context.Context
	Log      *slog.Logger
	Lc       fx.Lifecycle
	Chains   map[string]*config.Chain
	Cachers  cacher.Cachers
	Clusters cluster.Clusters
	Heads    vennstore.Headstores
	Election *election.Election
}

type Result struct {
	fx.Out

	Stalkers Stalkers
}

func New(p Params) (r Result, err error) {
	r.Stalkers, err = util.MakeMultichain(
		p.Chains,
		func(chain *config.Chain) (*Stalker, error) {
			if !chain.ParsedStalk {
				return nil, nil
			}

			store, err := util.GetChain(chain.Name, p.Heads)
			if err != nil {
				return nil, err
			}
			remote, err := util.ChooseChain2(chain.Name, p.Cachers, p.Clusters)
			if err != nil {
				return nil, err
			}

			s := &Stalker{
				ctx:      p.Ctx,
				log:      p.Log,
				chain:    chain,
				remote:   remote,
				store:    store,
				election: p.Election,
			}

			p.Lc.Append(fx.Hook{
				OnStart: func(_ context.Context) error {
					go s.start()
					return nil
				},
			})

			return s, nil
		},
	)
	return
}

func (T *Stalker) start() {
	T.election.RunWithLease(
		T.ctx,
		T.log.With("module", "stalker"),
		func(ctx context.Context) {
			T.stalk(ctx)
		},
		func(ctx context.Context) {
			<-ctx.Done()
		},
	)
}

func (T *Stalker) stalk(ctx context.Context) {
	for {
		waitfor, err := T.tick(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			T.log.Error("failed to get block head", "chain", T.chain.Name, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(waitfor):
			continue
		}
	}
}

func (T *Stalker) tick(ctx context.Context) (time.Duration, error) {
	blockTime := time.Duration(T.chain.BlockTimeSeconds * float64(time.Second))
	// ask for the latest block
	var block json.RawMessage
	if err := jrpcutil.Do(ctx, T.remote, &block, "eth_getBlockByNumber", []any{"latest", false}); err != nil {
		return blockTime, err
	}
	now := time.Now()

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

	prev, err := T.store.Put(ctx, head.BlockNumber)
	_ = prev
	if err != nil {
		// store error, so lets just wait the block time
		return blockTime, err
	}
	if prev == head.BlockNumber {
		// if there was no change, but its okay because the next one is yet to arrive
		if nextTime.After(now) {
			T.log.Debug("requested block too early stale block",
				"chain", T.chain.Name,
				"next", nextTime, "now", now,
			)
			return min(max(nextTime.Sub(now), 500*time.Millisecond), blockTime), nil
		}
		// in the case there is propogation delay, we take the max of 500ms and the blocktime/4, to avoid spamming nodes
		nextWait := max(500*time.Millisecond, blockTime/4)
		T.log.Debug("received stale block",
			"chain", T.chain.Name,
			"got", head.BlockNumber, "prev", prev,
			"expected time", nextTime, "now", now,
			"next wait", nextWait,
		)
		return nextWait, nil
		// otherwise, use the time until the expected time, or 500ms, whichever is greater
	} else {
		// we got a new block, so we can just use the smaller of these two values
		nextWait := max(min(nextTime.Sub(now), blockTime), 500*time.Millisecond, blockTime/4)
		T.log.Debug("received new block",
			"chain", T.chain.Name,
			"got", head.BlockNumber, "prev", prev,
			"expected time", nextTime, "now", now,
			"next wait", nextWait,
		)
		return nextWait, err
	}
}

func (T *Stalker) toConcreteBlockNumber(ctx context.Context, blockNumbers ...*ethtypes.BlockNumber) (changed bool, passthrough bool, err error) {
	var head hexutil.Uint64
	var hasHead bool
	for _, blockNumber := range blockNumbers {
		switch *blockNumber {
		case ethtypes.LatestBlockNumber:
			if !hasHead {
				head, err = T.store.Get(ctx)
				if err != nil {
					return
				}
				hasHead = true
			}

			*blockNumber = ethtypes.BlockNumber(head)
			changed = true
		case ethtypes.LatestExecutedBlockNumber, ethtypes.PendingBlockNumber, ethtypes.SafeBlockNumber, ethtypes.FinalizedBlockNumber:
			//		err = jsonrpc.NewInvalidParamsError(`expected "latest" or number`) // TODO(garet)
			return false, true, nil
		default:
			continue
		}
	}

	return
}

func (T *Stalker) ServeRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
	switch r.Method {
	case "eth_chainId":
		_ = w.Send(hexutil.Uint64(T.chain.Id), nil)
		return
	case "net_version":
		_ = w.Send(strconv.Itoa(T.chain.Id), nil)
		return
	case "eth_blockNumber":
		_ = w.Send(T.store.Get(r.Context()))
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

		changed, _, err := T.toConcreteBlockNumber(r.Context(), &blockNumber)
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

		T.remote.ServeRPC(w, r)
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

		changed, _, err := T.toConcreteBlockNumber(r.Context(), &request[0])
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

		T.remote.ServeRPC(w, r)
		return
	case "eth_call":
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
		if err := json.Unmarshal(request[1], &blockNumber); err != nil {
			_ = w.Send(nil, err)
			return
		}

		changed, _, err := T.toConcreteBlockNumber(r.Context(), &blockNumber)
		if err != nil {
			_ = w.Send(nil, err)
			return
		}

		if changed {
			r.Params, err = json.Marshal([]any{request[0], blockNumber})
			if err != nil {
				_ = w.Send(nil, err)
				return
			}
		}

		T.remote.ServeRPC(w, r)
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
			T.remote.ServeRPC(w, r)
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
			changed, _, err := T.toConcreteBlockNumber(r.Context(), &fromBlock, &toBlock)
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

			T.remote.ServeRPC(w, r)
			return
		}
	default:
		T.remote.ServeRPC(w, r)
		return
	}
}

var _ jrpc.Handler = (*Stalker)(nil)
