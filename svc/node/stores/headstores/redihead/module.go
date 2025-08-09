package redihead

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"sync"
	"time"

	"github.com/dranikpg/gtrs"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/svc/shared/services/redi"
)

type Head struct {
	Value uint64
	Chain string
}

type Params struct {
	fx.In

	Log    *slog.Logger
	Ctx    context.Context
	Redi   *redi.Redis `optional:"true"`
	Chains map[string]*config.Chain
}

type Result struct {
	fx.Out

	Result *Redihead `optional:"true"`
}

type Redihead struct {
	log    *slog.Logger
	redi   *redi.Redis
	ctx    context.Context
	stream gtrs.Stream[Head]

	head   map[string]uint64
	headMu sync.RWMutex

	subs map[string]map[int]chan<- hexutil.Uint64
	next int
	mu   sync.Mutex
}

func New(params Params) (r Result, err error) {
	if params.Redi == nil {
		params.Log.Info("redihead disabled", "reason", "no redis")
		return
	}

	stream := gtrs.NewStream[Head](
		params.Redi.C(),
		fmt.Sprintf("%s:head:stream", params.Redi.Namespace()),
		nil,
	)
	r.Result = &Redihead{
		log:    params.Log,
		ctx:    params.Ctx,
		redi:   params.Redi,
		stream: stream,
		head:   make(map[string]uint64),
	}
	go r.Result.start()
	return
}

func (T *Redihead) Get(ctx context.Context, chain *config.Chain) (hexutil.Uint64, error) {
	T.headMu.RLock()
	defer T.headMu.RUnlock()
	val, ok := T.head[chain.Name]
	if !ok {
		val = 0
	}
	return hexutil.Uint64(val), nil
}

func (T *Redihead) Put(ctx context.Context, chain *config.Chain, head hexutil.Uint64) (hexutil.Uint64, error) {
	was, err := T.Get(ctx, chain)
	if err != nil {
		return 0, err
	}
	// Optimistically update the in-memory head immediately so readers see fresh values
	T.setHead(chain.Name, uint64(head))
	_, err = T.stream.Add(ctx, Head{
		Value: uint64(head),
		Chain: chain.Name,
	})
	return was, err
}

func (T *Redihead) setHead(chainName string, head uint64) {
	T.headMu.Lock()
	cur := T.head[chainName]
	if head <= cur {
		T.headMu.Unlock()
		return
	}
	T.head[chainName] = head
	T.headMu.Unlock()
	func() {
		T.mu.Lock()
		defer T.mu.Unlock()
		chainSubs, ok := T.subs[chainName]
		if !ok {
			return
		}
		for _, sub := range chainSubs {
			select {
			case sub <- hexutil.Uint64(head):
			default:
			}
		}
	}()
}

func (T *Redihead) run(ctx context.Context) {
	messages, err := T.stream.RevRange(
		ctx,
		"+",
		"-",
		1,
	)
	if err != nil {
		T.log.Error("failed to get head", "error", err)
		return
	}

	var start string
	if len(messages) > 0 {
		T.setHead(messages[0].Data.Chain, messages[0].Data.Value)
		start = messages[0].ID
	} else {
		start = "$"
	}

	consume := func() bool {
		consumer := gtrs.NewConsumer[Head](
			ctx,
			T.redi.C(),
			gtrs.StreamIDs{
				fmt.Sprintf("%s:head:stream", T.redi.Namespace()): start,
			},
		)
		defer consumer.Close()

		for {
			select {
			case <-ctx.Done():
				return false
			case msg, ok := <-consumer.Chan():
				if !ok {
					log.Println("gtrs consumer closed. attempting reconnect in 1 second")
					time.Sleep(1 * time.Second)
					return true
				}
				if msg.Err != nil {
					T.log.Error("message error", "error", msg.Err)
					continue
				}

				T.setHead(msg.Data.Chain, msg.Data.Value)
			}
		}
	}
	for consume() {
	}
}

func (T *Redihead) start() {
	for {
		T.run(T.ctx)
		select {
		case <-T.ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (T *Redihead) On(chain *config.Chain) (<-chan hexutil.Uint64, func()) {
	T.mu.Lock()
	defer T.mu.Unlock()

	id := T.next
	T.next++

	if T.subs == nil {
		T.subs = make(map[string]map[int]chan<- hexutil.Uint64)
	}
	if _, ok := T.subs[chain.Name]; !ok {
		T.subs[chain.Name] = make(map[int]chan<- hexutil.Uint64)
	}
	sub := make(chan hexutil.Uint64, 1)
	T.subs[chain.Name][id] = sub
	return sub, func() {
		T.mu.Lock()
		defer T.mu.Unlock()

		if _, ok := T.subs[chain.Name][id]; ok {
			delete(T.subs[chain.Name], id)
			close(sub)
		}
	}
}

var _ headstore.Store = (*Redihead)(nil)
