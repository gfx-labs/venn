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
	blockTime := max(time.Second, time.Duration(T.chain.BlockTimeSeconds*float64(time.Second))/2)
	ticker := time.NewTicker(blockTime)
	defer ticker.Stop()
	for {
		err := T.tick(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			T.log.Error("failed to get block head", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			continue
		}
	}
}

func (T *Stalker) tick(ctx context.Context) error {
	var block json.RawMessage
	if err := jrpcutil.Do(ctx, T.remote, &block, "eth_getBlockByNumber", []any{"latest", false}); err != nil {
		return err
	}

	// extract block number
	var head struct {
		BlockNumber hexutil.Uint64 `json:"number"`
	}
	if err := json.Unmarshal(block, &head); err != nil {
		return err
	}

	return T.store.Put(ctx, head.BlockNumber)
}

func (T *Stalker) toConcreteBlockNumber(ctx context.Context, blockNumbers ...*ethtypes.BlockNumber) (changed bool, err error) {
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
			err = jsonrpc.NewInvalidParamsError(`expected "latest" or number`) // TODO(garet)
			return
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

		changed, err := T.toConcreteBlockNumber(r.Context(), &blockNumber)
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

		changed, err := T.toConcreteBlockNumber(r.Context(), &request[0])
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

		changed, err := T.toConcreteBlockNumber(r.Context(), &blockNumber)
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
			changed, err := T.toConcreteBlockNumber(r.Context(), &fromBlock, &toBlock)
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
