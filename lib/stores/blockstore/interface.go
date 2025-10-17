package blockstore

import (
	"context"
	"encoding/json"
	"github.com/gfx-labs/venn/lib/config"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type EntryType int

const (
	EntryBlockHeader EntryType = iota
	EntryLogs
	EntryReceipts

	EntryTypeCount
)

type Entry struct {
	BlockHash   common.Hash    `json:"blockHash"`
	BlockNumber hexutil.Uint64 `json:"blockNumber"`

	ParentHash *common.Hash `json:"parentHash"`

	Value json.RawMessage `json:"value"`
}

type Query interface {
	query()
}

type QueryRange struct {
	Start hexutil.Uint64
	End   hexutil.Uint64
}

func (QueryRange) query() {}

var _ Query = QueryRange{}

type QueryHash common.Hash

func (QueryHash) query() {}

var _ Query = QueryHash{}

func QueryNumber(n hexutil.Uint64) QueryRange {
	return QueryRange{
		Start: n,
		End:   n,
	}
}

type Store interface {
	Get(ctx context.Context, chain *config.Chain, typ EntryType, query Query) ([]*Entry, error)
	Put(ctx context.Context, chain *config.Chain, typ EntryType, entries ...*Entry) error
}
