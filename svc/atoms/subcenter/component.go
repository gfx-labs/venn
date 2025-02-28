package subcenter

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/go-faster/jx"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/ethtypes"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/lib/util"
	"gfx.cafe/gfx/venn/svc/atoms/vennstore"
)

type Subcenters = util.Multichain[*Subcenter]

type Subcenter struct {
	store headstore.Store
	log   *slog.Logger
}

type Params struct {
	fx.In

	Log    *slog.Logger
	Heads  vennstore.Headstores
	Chains map[string]*config.Chain
}

type Result struct {
	fx.Out

	Result Subcenters `optional:"true"`
}

func New(p Params) (r Result, err error) {
	r.Result, err = util.MakeMultichain(
		p.Chains,
		func(chain *config.Chain) (*Subcenter, error) {
			head, err := util.GetChain(chain.Name, p.Heads)
			if err != nil {
				return nil, err
			}

			return &Subcenter{
				store: head,
				log:   p.Log.With("chain", chain.Name),
			}, nil
		},
	)
	return
}

func (T *Subcenter) Track(ctx context.Context) {
	ch, done := T.store.On()
	defer done()

	for {
		select {
		case <-ctx.Done():
			return
		case head := <-ch:
			T.log.Info("HEAD", "number", head)
		}
	}
}

func (T *Subcenter) removeTransactions(block json.RawMessage) (json.RawMessage, error) {
	d := jx.DecodeBytes(block)
	var w jx.Writer

	header, err := d.ObjIter()
	if err != nil {
		return nil, err
	}
	w.ObjStart()
	first := true
	for header.Next() {
		if bytes.Equal(header.Key(), []byte("transactions")) {
			if err = d.Skip(); err != nil {
				return nil, err
			}
			continue
		}

		if first {
			first = false
		} else {
			w.Comma()
		}
		w.ByteStr(header.Key())
		w.RawStr(":")

		raw, err := d.Raw()
		if err != nil {
			return nil, err
		}
		w.Raw(raw)
	}
	w.ObjEnd()

	return w.Buf, nil
}

func (T *Subcenter) Middleware(h jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		switch r.Method {
		case "eth_subscribe":
			notifier, ok := subscription.NotifierFromContext(r.Context())
			if !ok {
				_ = w.Send(nil, subscription.ErrNotificationsUnsupported)
				return
			}

			var params []json.RawMessage
			if err := json.Unmarshal(r.Params, &params); err != nil {
				_ = w.Send(nil, err)
				return
			}

			if len(params) == 0 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected at least one param"))
				return
			}

			var method string
			if err := json.Unmarshal(params[0], &method); err != nil {
				_ = w.Send(nil, err)
				return
			}

			params = params[1:]

			current, err := T.store.Get(r.Context())
			if err != nil {
				_ = w.Send(nil, err)
				return
			}

			// all good, so we can send the id

			switch method {
			case "newHeads":
				{
					sub, done := T.store.On()
					defer done()

					w.Send(notifier.ID(), nil)
					for {
						select {
						case <-r.Context().Done():
							T.log.Info("context closed. closing subscription")
							return
						case err := <-notifier.Err():
							T.log.Error("notifier error. subscription closing.", "error", err)
							return
						case head := <-sub:
							// NOTE: eth_subscribe doesn't guarantee that every single block will be sent.
							// we could implement native retry logic the underlying cluster, retrying non application/user errors up to N times with some sort of backoff, to better deliver blocks.
							for i := current + 1; i <= head; i++ {
								var block json.RawMessage
								if err := jrpcutil.Do(r.Context(), h, &block, "eth_getBlockByNumber", []any{i, false}); err != nil {
									T.log.Error("failed to get block", "error", err)
									continue
								}
								withoutTxns, err := T.removeTransactions(block)
								if err != nil {
									T.log.Error("failed to remove txns from block. was the block invalid?", "error", err)
									continue
								}
								// if the notifier errors, the connection should closed if it errors anyways, so we can error here and return, since that will happen anyways, we may as well not waste the calls to more blocks
								if err := notifier.Notify(withoutTxns); err != nil {
									T.log.Error("failed to notify the subscription", "error", err)
									break
								}
							}
							current = head
						}
					}
				}
			case "logs":
				{
					var filter ethtypes.SubscriptionFilterQuery
					if len(params) != 1 {
						_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 1 parameter"))
						return
					}

					if err = json.Unmarshal(params[0], &filter); err != nil {
						_ = w.Send(nil, jsonrpc.NewInvalidParamsError(err.Error()))
						return
					}
					sub, done := T.store.On()
					defer done()
					w.Send(notifier.ID(), nil)
					for {
						select {
						case <-r.Context().Done():
							T.log.Info("context closed. closing subscription")
							return
						case err = <-notifier.Err():
							T.log.Error("notifier error. subscription closing.", "error", err)
							return
						case head := <-sub:
							from := ethtypes.BlockNumber(current + 1)
							to := ethtypes.BlockNumber(head)

							var logs json.RawMessage
							if err := jrpcutil.Do(r.Context(), h, &logs, "eth_getLogs", []any{
								ethtypes.FilterQuery{
									FromBlock: &from,
									ToBlock:   &to,
									Addresses: filter.Addresses,
									Topics:    filter.Topics,
								},
							}); err != nil {
								T.log.Error("failed to get logs for sub", "error", err)
								continue
							}

							d := jx.DecodeBytes(logs)
							arr, err := d.ArrIter()
							if err != nil {
								T.log.Error("failed to decode logs. are the logs corrupt?", "error", err)
								continue
							}
							for arr.Next() {
								log, err := d.Raw()
								if err != nil {
									T.log.Error("failed to decode log. are they corrupt?", "error", err)
									continue
								}
								if err = notifier.Notify(json.RawMessage(log)); err != nil {
									T.log.Error("error notifying subscription", "error", err)
									break
								}
							}

							current = head
						}
					}
				}
			default:
				_ = w.Send(nil, jsonrpc.NewInvalidRequestError("unknown subscription method"))
				return
			}
		default:
			h.ServeRPC(w, r)
			return
		}
	})
}
