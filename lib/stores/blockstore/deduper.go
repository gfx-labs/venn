package blockstore

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/singleflight"
)

type Deduper struct {
	underlying Store
	group      singleflight.Group
}

func NewDeduper(underlying Store) *Deduper {
	return &Deduper{
		underlying: underlying,
	}
}

func (T *Deduper) Get(ctx context.Context, typ EntryType, query Query) ([]*Entry, error) {
	var groupQuery string
	switch q := query.(type) {
	case QueryRange:
		groupQuery = fmt.Sprintf("%d.%v.%v", typ, q.Start, q.End)
	case QueryHash:
		groupQuery = fmt.Sprintf("%d.%s", typ, common.Hash(q).Hex())
	}
	result := T.group.DoChan(groupQuery, func() (interface{}, error) {
		return T.underlying.Get(ctx, typ, query)
	})
	select {
	case res := <-result:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.([]*Entry), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (T *Deduper) Put(ctx context.Context, typ EntryType, entries ...*Entry) error {
	return T.underlying.Put(ctx, typ, entries...)
}

var _ Store = (*Deduper)(nil)
