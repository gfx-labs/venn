package callcenter

import (
	"context"
	"encoding/json"
	"strings"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/bytedance/sonic"
)

type Proxier struct {
	connect func(ctx context.Context) (jrpc.Conn, error)
	conn    subscription.Conn
}

func NewProxier(connect func(ctx context.Context) (jrpc.Conn, error)) *Proxier {
	return &Proxier{
		connect: connect,
	}
}

func init() {
	subscription.SetServiceMethodSeparator("_")
}

func (T *Proxier) ServeRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
	var ok bool
	if T.conn != nil {
		select {
		case <-T.conn.Closed():
		default:
			ok = true
		}
	}

	if !ok {
		var err error
		T.conn, err = subscription.UpgradeConn(T.connect(r.Context()))
		if err != nil {
			_ = w.Send(nil, err)
			return
		}
	}

	if strings.HasSuffix(r.Method, "_subscribe") {
		notifier, ok := subscription.NotifierFromContext(r.Context())
		if !ok {
			_ = w.Send(nil, subscription.ErrNotificationsUnsupported)
		}
		ch := make(chan json.RawMessage)
		sub, err := T.conn.Subscribe(r.Context(), strings.TrimSuffix(r.Method, "_subscribe"), ch, r.Params)
		if err != nil {
			_ = w.Send(nil, err)
			return
		}

		go func() {
			defer func() {
				_ = sub.Unsubscribe()
			}()

			for {
				select {
				case data := <-ch:
					err = notifier.Notify(data)
					if err != nil {
						return
					}
				case <-sub.Err():
					return
				case <-notifier.Err():
					return
				}
			}
		}()
		return
	}

	params := r.Params
	if len(params) == 0 {
		params = nil
	}
	var result sonic.NoCopyRawMessage
	err := T.conn.Do(r.Context(), &result, r.Method, r.Params)

	_ = w.Send(result, err)
}

func (T *Proxier) Close() error {
	return T.conn.Close()
}

var _ Remote = (*Proxier)(nil)
