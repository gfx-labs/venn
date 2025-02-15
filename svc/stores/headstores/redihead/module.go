package redihead

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dranikpg/gtrs"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/lib/util"
	"gfx.cafe/gfx/venn/svc/services/redi"
)

type Head struct {
	Value uint64
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

	Result Rediheads `optional:"true"`
}

type Rediheads = util.Multichain[*Redihead]

type Redihead struct {
	chain  *config.Chain
	log    *slog.Logger
	redi   *redi.Redis
	ctx    context.Context
	stream gtrs.Stream[Head]

	head atomic.Uint64

	subs map[int]chan<- hexutil.Uint64
	next int
	mu   sync.Mutex
}

func New(params Params) (r Result, err error) {
	if params.Redi == nil {
		params.Log.Info("redihead disabled", "reason", "no redis")
		return
	}
	r.Result, err = util.MakeMultichain(
		params.Chains,
		func(chain *config.Chain) (*Redihead, error) {
			stream := gtrs.NewStream[Head](
				params.Redi.C(),
				fmt.Sprintf("venn:%s:%s:head:stream", params.Redi.Namespace(), chain.Name),
				nil,
			)

			redihead := &Redihead{
				chain:  chain,
				log:    params.Log,
				ctx:    params.Ctx,
				redi:   params.Redi,
				stream: stream,
			}

			go redihead.start()

			return redihead, nil
		},
	)
	return
}

func (T *Redihead) Get(ctx context.Context) (hexutil.Uint64, error) {
	return hexutil.Uint64(T.head.Load()), nil
}

func (T *Redihead) Put(ctx context.Context, head hexutil.Uint64) (hexutil.Uint64, error) {
	was, err := T.Get(ctx)
	if err != nil {
		return 0, err
	}
	_, err = T.stream.Add(ctx, Head{
		Value: uint64(head),
	})
	return was, err
}

func (T *Redihead) setHead(head uint64) {
	T.head.Store(head)

	func() {
		T.mu.Lock()
		defer T.mu.Unlock()

		for _, sub := range T.subs {
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
		T.setHead(messages[0].Data.Value)
		start = messages[0].ID
	} else {
		start = "$"
	}

	consume := func() bool {
		consumer := gtrs.NewConsumer[Head](
			ctx,
			T.redi.C(),
			gtrs.StreamIDs{
				fmt.Sprintf("venn:%s:%s:head:stream", T.redi.Namespace(), T.chain.Name): start,
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

				T.setHead(msg.Data.Value)
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

func (T *Redihead) On() (<-chan hexutil.Uint64, func()) {
	T.mu.Lock()
	defer T.mu.Unlock()

	id := T.next
	T.next++

	if T.subs == nil {
		T.subs = make(map[int]chan<- hexutil.Uint64)
	}
	sub := make(chan hexutil.Uint64, 1)
	T.subs[id] = sub

	return sub, func() {
		T.mu.Lock()
		defer T.mu.Unlock()

		if _, ok := T.subs[id]; ok {
			delete(T.subs, id)
			close(sub)
		}
	}
}

var _ headstore.Store = (*Redihead)(nil)
