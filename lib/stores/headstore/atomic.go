package headstore

import (
	"context"
	"sync"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Atomic struct {
	head hexutil.Uint64
	subs map[int]chan<- hexutil.Uint64
	next int
	mu   sync.RWMutex
}

func NewAtomic() *Atomic {
	return new(Atomic)
}

func (T *Atomic) Get(_ context.Context) (hexutil.Uint64, error) {
	T.mu.RLock()
	defer T.mu.RUnlock()

	return T.head, nil
}

func (T *Atomic) Put(_ context.Context, head hexutil.Uint64) error {
	T.mu.Lock()
	defer T.mu.Unlock()

	if T.head >= head {
		return nil
	}

	for _, sub := range T.subs {
		select {
		case sub <- head:
		default:
		}
	}

	T.head = head

	return nil
}

func (T *Atomic) On() (<-chan hexutil.Uint64, func()) {
	T.mu.Lock()
	defer T.mu.Unlock()

	id := T.next
	T.next++

	if T.subs == nil {
		T.subs = make(map[int]chan<- hexutil.Uint64)
	}
	ch := make(chan hexutil.Uint64, 1)
	T.subs[id] = ch

	return ch, func() {
		T.mu.Lock()
		defer T.mu.Unlock()

		if _, ok := T.subs[id]; ok {
			delete(T.subs, id)
			close(ch)
		}
	}
}

var _ Store = (*Atomic)(nil)
