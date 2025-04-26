package headstore

import (
	"context"
	"gfx.cafe/gfx/venn/lib/config"
	"sync"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Atomic struct {
	heads map[string]hexutil.Uint64
	subs  map[string]map[int]chan<- hexutil.Uint64
	next  int
	mu    sync.RWMutex
}

func NewAtomic() *Atomic {
	return new(Atomic)
}

func (T *Atomic) Get(_ context.Context, chain *config.Chain) (hexutil.Uint64, error) {
	T.mu.RLock()
	defer T.mu.RUnlock()
	val, ok := T.heads[chain.Name]
	if !ok {
		return 0, nil
	}
	return val, nil
}

func (T *Atomic) Put(_ context.Context, chain *config.Chain, head hexutil.Uint64) (prev hexutil.Uint64, err error) {
	T.mu.Lock()
	defer T.mu.Unlock()

	cur := T.heads[chain.Name]

	if cur >= head {
		return cur, nil
	}

	if va, ok := T.subs[chain.Name]; ok {
		for _, sub := range va {
			select {
			case sub <- head:
			default:
			}
		}
	}
	T.heads[chain.Name] = head
	return cur, nil
}

func (T *Atomic) On(chain *config.Chain) (<-chan hexutil.Uint64, func()) {
	T.mu.Lock()
	defer T.mu.Unlock()

	id := T.next
	T.next++

	if T.subs == nil {
		T.subs = make(map[string]map[int]chan<- hexutil.Uint64)
	}
	if T.subs[chain.Name] == nil {
		T.subs[chain.Name] = make(map[int]chan<- hexutil.Uint64)
	}
	ch := make(chan hexutil.Uint64, 1)
	T.subs[chain.Name][id] = ch

	return ch, func() {
		T.mu.Lock()
		defer T.mu.Unlock()

		if _, ok := T.subs[chain.Name][id]; ok {
			delete(T.subs[chain.Name], id)
			close(ch)
		}
	}
}

var _ Store = (*Atomic)(nil)
