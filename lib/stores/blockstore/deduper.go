package blockstore

import (
	"context"
	"fmt"

	"github.com/gfx-labs/venn/lib/config"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/singleflight"
)

type SingleFlight struct {
	underlying Store
	group      singleflight.Group
}

func NewSingleFlight(underlying Store) *SingleFlight {
	return &SingleFlight{
		underlying: underlying,
	}
}

func (T *SingleFlight) Get(ctx context.Context, chain *config.Chain, typ EntryType, query Query) ([]*Entry, error) {
	var groupQuery string
	switch q := query.(type) {
	case QueryRange:
		groupQuery = fmt.Sprintf("%s.%d.%v.%v", chain.Name, typ, q.Start, q.End)
	case QueryHash:
		groupQuery = fmt.Sprintf("%s.%d.%s", chain.Name, typ, common.Hash(q).Hex())
	}
	result := T.group.DoChan(groupQuery, func() (any, error) {
		return T.underlying.Get(ctx, chain, typ, query)
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

func (T *SingleFlight) Put(ctx context.Context, chain *config.Chain, typ EntryType, entries ...*Entry) error {
	return T.underlying.Put(ctx, chain, typ, entries...)
}

var _ Store = (*SingleFlight)(nil)
