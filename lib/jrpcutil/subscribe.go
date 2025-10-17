package jrpcutil

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"

	"github.com/gfx-labs/venn/lib/subctx"
)

var engine = subscription.NewEngine()

type Subscription struct {
	channel   reflect.Value
	namespace string
	id        string

	error  chan error
	closed bool
	mu     sync.Mutex
}

func (T *Subscription) err(err error) {
	T.mu.Lock()
	defer T.mu.Unlock()
	if T.closed {
		return
	}
	select {
	case T.error <- err:
	default:
	}
}

func (T *Subscription) Err() <-chan error {
	return T.error
}

func (T *Subscription) Unsubscribe() error {
	T.mu.Lock()
	if T.closed {
		T.mu.Unlock()
		return nil
	}
	T.closed = true
	close(T.error)
	T.mu.Unlock()

	handler := engine.Middleware()(nil)

	r, err := jsonrpc.NewRequest(
		subctx.WithInternal(context.Background(), true),
		jsonrpc.NewNullIDPtr(),
		T.namespace+"_unsubscribe",
		[]any{
			T.id,
		},
	)
	if err != nil {
		return err
	}

	var resp Interceptor
	handler.ServeRPC(&resp, r)

	return resp.Error
}

func (T *Subscription) String() string {
	return T.id
}

func (T *Subscription) notify(v any) {
	var result struct {
		Result json.RawMessage `json:"result,omitempty"`
	}
	vb, err := json.Marshal(v)
	if err != nil {
		T.err(err)
		return
	}
	if err = json.Unmarshal(vb, &result); err != nil {
		T.err(err)
		return
	}

	val := reflect.New(T.channel.Type().Elem())
	err = json.Unmarshal(result.Result, val.Interface())
	if err != nil {
		T.err(err)
		return
	}
	reflect.Select([]reflect.SelectCase{
		{
			Dir:  reflect.SelectSend,
			Chan: T.channel,
			Send: val.Elem(),
		},
		{
			Dir: reflect.SelectDefault,
		},
	})
	return
}

var _ subscription.ClientSubscription = (*Subscription)(nil)

type subscriptionInterceptor struct {
	sub         *Subscription
	error       error
	extraFields jsonrpc.ExtraFields
}

func (T *subscriptionInterceptor) Send(id any, err error) error {
	if err != nil {
		T.error = err
		return nil
	}
	idb, err := json.Marshal(id)
	if err != nil {
		return err
	}
	err = json.Unmarshal(idb, &T.sub.id)
	if err != nil {
		return err
	}
	return nil
}

func (T *subscriptionInterceptor) Notify(_ string, v any) error {
	T.sub.notify(v)
	return nil
}

func (T *subscriptionInterceptor) ExtraFields() jsonrpc.ExtraFields {
	if T.extraFields == nil {
		T.extraFields = make(jsonrpc.ExtraFields)
	}
	return T.extraFields
}

var _ jrpc.ResponseWriter = (*subscriptionInterceptor)(nil)

func Subscribe(ctx context.Context, handler jrpc.Handler, namespace string, channel any, args any) (subscription.ClientSubscription, error) {
	chanVal := reflect.ValueOf(channel)
	// make sure its a proper channel
	chanVal.Kind()
	if chanVal.Kind() != reflect.Chan || chanVal.Type().ChanDir()&reflect.SendDir == 0 {
		panic("first argument to Subscribe must be a writable channel")
	}
	if chanVal.IsNil() {
		panic("channel given to Subscribe must not be nil")
	}

	r, err := jsonrpc.NewRequest(
		subctx.WithInternal(ctx, true),
		jsonrpc.NewNullIDPtr(),
		namespace+"_subscribe",
		args,
	)
	if err != nil {
		return nil, err
	}

	sub := &Subscription{
		channel:   chanVal,
		namespace: namespace,

		error: make(chan error),
	}
	interceptor := subscriptionInterceptor{
		sub: sub,
	}

	handler = engine.Middleware()(handler)
	handler.ServeRPC(&interceptor, r)

	if interceptor.error != nil {
		return nil, interceptor.error
	}

	return sub, nil
}
