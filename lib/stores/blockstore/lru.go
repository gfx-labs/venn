package blockstore

import (
	"context"
	"errors"
	"gfx.cafe/gfx/venn/lib/config"
	"sync"

	"gfx.cafe/util/go/generic"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/hashicorp/golang-lru/v2/simplelru"
)

type lruNumberKey struct {
	Type        EntryType
	BlockNumber hexutil.Uint64
	Chain       string
}

type lruHashKey struct {
	Type      EntryType
	BlockHash common.Hash
	Chain     string
}

type LruStore struct {
	size int

	byHash   *simplelru.LRU[lruHashKey, *Entry]
	byNumber *simplelru.LRU[lruNumberKey, *Entry]

	mu sync.Mutex
}

func NewLruStore(size int) *LruStore {
	return &LruStore{
		size:     size,
		byNumber: generic.Must(simplelru.NewLRU[lruNumberKey, *Entry](size, nil)),
		byHash:   generic.Must(simplelru.NewLRU[lruHashKey, *Entry](size, nil)),
	}
}

func (T *LruStore) Get(_ context.Context, chain *config.Chain, typ EntryType, query Query) ([]*Entry, error) {
	T.mu.Lock()
	defer T.mu.Unlock()

	switch q := query.(type) {
	case QueryHash:
		entry, ok := T.byHash.Get(lruHashKey{
			Type:      typ,
			BlockHash: common.Hash(q),
			Chain:     chain.Name,
		})
		if !ok {
			return nil, errors.New("not found")
		}

		return []*Entry{entry}, nil
	case QueryRange:
		entries := make([]*Entry, 0, q.End-q.Start+1)
		for i := q.Start; i <= q.End; i++ {
			entry, ok := T.byNumber.Get(lruNumberKey{
				Type:        typ,
				BlockNumber: i,
				Chain:       chain.Name,
			})
			if !ok {
				return nil, errors.New("not found")
			}
			entries = append(entries, entry)
		}

		return entries, nil
	default:
		return nil, errors.New("unknown query")
	}
}

func (T *LruStore) Put(_ context.Context, chain *config.Chain, typ EntryType, entries ...*Entry) error {
	T.mu.Lock()
	defer T.mu.Unlock()
	for _, entry := range entries {
		if entry.ParentHash != nil {
			// get prev
			if prev, ok := T.byNumber.Get(lruNumberKey{
				Type:        typ,
				BlockNumber: entry.BlockNumber - 1,
				Chain:       chain.Name,
			}); ok {
				if prev.BlockHash != *entry.ParentHash {
					T.byNumber.Purge()
				}
			}
		}

		T.byHash.Add(lruHashKey{
			Type:      typ,
			BlockHash: entry.BlockHash,
			Chain:     chain.Name,
		}, entry)
		T.byNumber.Add(lruNumberKey{
			Type:        typ,
			BlockNumber: entry.BlockNumber,
			Chain:       chain.Name,
		}, entry)
	}

	return nil
}

var _ Store = (*LruStore)(nil)
