package blockstore

import (
	"context"
	"errors"
	"log/slog"

	"go.uber.org/multierr"
)

func NewCompoundStore(log *slog.Logger) *CompoundStore {
	return &CompoundStore{log: log}
}

type CompoundStore struct {
	log    *slog.Logger
	stores []namedStore
}

type namedStore struct {
	name  string
	store Store
}

func (c *CompoundStore) AddStore(name string, store Store) {
	if store == nil {
		return
	}
	c.stores = append(c.stores, namedStore{
		name:  name,
		store: store,
	})
}

// Put the block in every store. it's up to the store implementations to deal with deduplication, etc
func (c *CompoundStore) Put(ctx context.Context, typ EntryType, entries ...*Entry) error {
	var merr error
	for _, v := range c.stores {
		err := v.store.Put(ctx, typ, entries...)
		if err != nil {
			merr = multierr.Append(merr, err)
		}
	}
	if merr != nil {
		return merr
	}
	return nil
}

func (c *CompoundStore) Get(ctx context.Context, typ EntryType, query Query) ([]*Entry, error) {
	if q, ok := query.(QueryRange); ok {
		if q.End < q.Start {
			return nil, nil
		}
	}

	var entries []*Entry
	var merr error
	var i int
	var v namedStore
	var ok bool
	for i, v = range c.stores {
		var err error
		entries, err = v.store.Get(ctx, typ, query)
		if err != nil {
			merr = multierr.Append(merr, err)
			continue
		}
		ok = true
		break
	}
	// the only reason blk should be nil is if merr is not nil
	if !ok {
		if merr == nil {
			return nil, errors.New("compound handler could not find entries")
		}
		return nil, merr
	}
	merr = nil
	for j := 0; j < i; j++ {
		v := c.stores[j]
		err := v.store.Put(ctx, typ, entries...)
		if err != nil {
			if !(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				c.log.Warn("cache miss", "cachetype", v.name, "query", query, "err", err)
			}
			merr = multierr.Append(merr, err)
			continue
		}
	}

	if merr != nil {
		return nil, merr
	}
	return entries, nil
}

var _ Store = (*CompoundStore)(nil)
