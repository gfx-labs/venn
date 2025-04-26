package callcenter

import (
	"context"
	"io"

	"gfx.cafe/open/jrpc"
	"github.com/jackc/puddle"
)

// Pooler will reuse connections.
type Pooler struct {
	pool *puddle.Pool
}

func NewPooler(new func(ctx context.Context) (Remote, error), maxSize int) *Pooler {
	pool := puddle.NewPool(
		func(ctx context.Context) (res interface{}, err error) {
			return new(ctx)
		},
		func(res interface{}) {
			cast, ok := res.(io.Closer)
			if !ok {
				return
			}
			_ = cast.Close()
		},
		int32(maxSize),
	)

	return &Pooler{
		pool: pool,
	}
}

func (T *Pooler) ServeRPC(w jrpc.ResponseWriter, r *jrpc.Request) {
	handler, err := T.pool.Acquire(r.Context())
	if err != nil {
		_ = w.Send(nil, err)
		return
	}
	defer handler.Release()
	remote := handler.Value().(Remote)

	remote.ServeRPC(w, r)
}

func (T *Pooler) Close() error {
	T.pool.Close()
	return nil
}

var _ Remote = (*Pooler)(nil)
