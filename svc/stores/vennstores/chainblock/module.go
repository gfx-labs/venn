package chainblock

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-faster/jx"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/lib/ethtypes"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/stores/blockstore"
	"gfx.cafe/gfx/venn/lib/subctx"
	"gfx.cafe/gfx/venn/lib/util"
	"gfx.cafe/gfx/venn/svc/quarks/cluster"
)

type Chainblocks = util.Multichain[*Chainblock]

type Chainblock struct {
	remote callcenter.Remote

	log *slog.Logger
}

type Params struct {
	fx.In

	Clusters cluster.Clusters

	Log *slog.Logger
}

type Result struct {
	fx.Out

	Output Chainblocks
}

func New(p Params) (r Result, err error) {
	r.Output, err = util.MakeMultichain(
		p.Clusters,
		func(remote callcenter.Remote) (*Chainblock, error) {
			return &Chainblock{
				remote: remote,
				log:    p.Log,
			}, nil
		},
	)
	return
}

func (T *Chainblock) Put(_ context.Context, _ blockstore.EntryType, _ ...*blockstore.Entry) error {
	return nil
}

func (T *Chainblock) Get(ctx context.Context, typ blockstore.EntryType, query blockstore.Query) ([]*blockstore.Entry, error) {
	switch typ {
	case blockstore.EntryBlockHeader:
		return T.getBlockHeaders(ctx, query)
	case blockstore.EntryReceipts:
		return T.getReceipts(ctx, query)
	case blockstore.EntryLogs:
		return T.getLogs(ctx, query)
	default:
		return nil, errors.New("unknown entry type")
	}
}

func (T *Chainblock) getBlockHeaders(ctx context.Context, query blockstore.Query) ([]*blockstore.Entry, error) {
	switch q := query.(type) {
	case blockstore.QueryHash:
		var out blockstore.Entry
		if err := jrpcutil.Do(ctx, T.remote, &out.Value, "eth_getBlockByHash", []any{common.Hash(q), true}); err != nil {
			return nil, err
		}

		var head struct {
			BlockHash   common.Hash    `json:"hash"`
			BlockNumber hexutil.Uint64 `json:"number"`
			ParentHash  common.Hash    `json:"parentHash"`
		}

		if err := json.Unmarshal(out.Value, &head); err != nil {
			return nil, err
		}

		if head.BlockHash != common.Hash(q) {
			return nil, nil
		}

		out.BlockHash = common.Hash(q)
		out.ParentHash = &head.ParentHash
		out.BlockNumber = head.BlockNumber

		return []*blockstore.Entry{&out}, nil
	case blockstore.QueryRange:
		if q.End-q.Start < 0 {
			return nil, nil
		}

		out := make([]*blockstore.Entry, q.End-q.Start+1)
		for i := q.Start; i <= q.End; i++ {
			var res blockstore.Entry
			if err := jrpcutil.Do(ctx, T.remote, &res.Value, "eth_getBlockByNumber", []any{i, true}); err != nil {
				return nil, err
			}

			var head struct {
				BlockHash  common.Hash `json:"hash"`
				ParentHash common.Hash `json:"parentHash"`
			}

			if err := json.Unmarshal(res.Value, &head); err != nil {
				return nil, err
			}

			if head.BlockHash == (common.Hash{}) {
				return nil, nil
			}

			res.BlockHash = head.BlockHash
			res.ParentHash = &head.ParentHash
			res.BlockNumber = i

			out[i-q.Start] = &res
		}

		return out, nil
	default:
		return nil, errors.New("unknown query")
	}
}

func (T *Chainblock) getReceipts(ctx context.Context, query blockstore.Query) ([]*blockstore.Entry, error) {
	switch q := query.(type) {
	case blockstore.QueryHash:
		return nil, errors.New("cannot get block receipts by hash")
	case blockstore.QueryRange:
		if q.End-q.Start < 0 {
			return nil, nil
		}

		out := make([]*blockstore.Entry, q.End-q.Start+1)
		for i := q.Start; i <= q.End; i++ {
			var res blockstore.Entry
			if err := jrpcutil.Do(ctx, T.remote, &res.Value, "eth_getBlockReceipts", []any{i}); err != nil {
				return nil, err
			}

			var head [1]struct {
				BlockHash common.Hash `json:"blockHash"`
			}

			if err := json.Unmarshal(res.Value, &head); err != nil {
				return nil, err
			}

			if head[0].BlockHash == (common.Hash{}) {
				return nil, nil
			}

			res.BlockHash = head[0].BlockHash
			res.BlockNumber = i

			out[i-q.Start] = &res
		}

		return out, nil
	default:
		return nil, errors.New("unknown query")
	}
}

func (T *Chainblock) getLogs(ctx context.Context, query blockstore.Query) ([]*blockstore.Entry, error) {
	switch q := query.(type) {
	case blockstore.QueryHash:
		var out blockstore.Entry
		hash := common.Hash(q)

		logsMethod := "eth_getLogs"

		chain, err := subctx.GetChain(ctx)
		if err == nil {
			if chain == "sei" {
				logsMethod = "sei_getLogs"
			}
		}

		if err := jrpcutil.Do(ctx, T.remote, &out.Value, logsMethod, []any{
			ethtypes.FilterQuery{
				BlockHash: &hash,
			},
		}); err != nil {
			return nil, err
		}

		var head [1]struct {
			BlockHash   common.Hash    `json:"blockHash"`
			BlockNumber hexutil.Uint64 `json:"blockNumber"`
		}

		if err := json.Unmarshal(out.Value, &head); err != nil {
			return nil, err
		}

		if head[0].BlockHash != hash {
			return nil, nil
		}

		out.BlockHash = hash
		out.BlockNumber = head[0].BlockNumber

		return []*blockstore.Entry{&out}, nil
	case blockstore.QueryRange:
		if q.End-q.Start < 0 {
			return nil, nil
		}

		fromBlock := ethtypes.BlockNumber(q.Start)
		toBlock := ethtypes.BlockNumber(q.End)

		logsMethod := "eth_getLogs"

		chain, err := subctx.GetChain(ctx)
		if err == nil {
			if chain == "sei" {
				logsMethod = "sei_getLogs"
			}
		}

		var logs json.RawMessage
		if err := jrpcutil.Do(ctx, T.remote, &logs, logsMethod, []any{
			ethtypes.FilterQuery{
				FromBlock: &fromBlock,
				ToBlock:   &toBlock,
			},
		}); err != nil {
			return nil, err
		}

		if fromBlock == toBlock {
			// single block shortcut

			var head [1]struct {
				BlockHash common.Hash `json:"blockHash"`
			}

			if err := json.Unmarshal(logs, &head); err != nil {
				return nil, err
			}

			// no logs
			if head[0].BlockHash == (common.Hash{}) {
				return nil, nil
			}

			return []*blockstore.Entry{
				{
					BlockNumber: q.Start,
					BlockHash:   head[0].BlockHash,

					Value: logs,
				},
			}, nil
		}

		encoders := make([]jx.Writer, q.End-q.Start+1)
		results := make([]*blockstore.Entry, q.End-q.Start+1)
		d := jx.DecodeBytes(logs)

		arr, err := d.ArrIter()
		if err != nil {
			return nil, err
		}

		for arr.Next() {
			raw, err := d.Raw()
			if err != nil {
				return nil, err
			}

			var header struct {
				BlockHash   common.Hash    `json:"blockHash"`
				BlockNumber hexutil.Uint64 `json:"blockNumber"`
			}
			if err = json.Unmarshal(raw, &header); err != nil {
				return nil, err
			}

			i := int(header.BlockNumber - q.Start)
			if i < 0 || i >= len(encoders) {
				continue
			}

			encoder := &encoders[i]
			if len(encoder.Buf) == 0 {
				encoder.ArrStart()
			} else {
				encoder.Comma()
			}
			encoder.Raw(raw)

			res := results[i]
			if res == nil {
				res = &blockstore.Entry{
					BlockHash:   header.BlockHash,
					BlockNumber: header.BlockNumber,
				}
				results[i] = res
			}
		}

		j := 0
		for i, res := range results {
			if res != nil {
				encoders[i].ArrEnd()
				res.Value = encoders[i].Buf
				results[j] = res
				j += 1
			}
		}
		results = results[:j]

		return results, nil
	default:
		return nil, errors.New("unknown query")
	}
}

var _ blockstore.Store = (*Chainblock)(nil)
