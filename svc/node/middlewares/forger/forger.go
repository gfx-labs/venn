package forger

import (
	"bytes"
	"encoding/json"
	"gfx.cafe/open/jrpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-faster/jx"
	"golang.org/x/sync/errgroup"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/ethtypes"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/subctx"
)

type Forger struct {
	Chains map[string]*config.Chain
}

func (T *Forger) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		switch r.Method {
		case "eth_getBlockReceipts":
			// Check if forging is enabled for this chain
			chain, err := subctx.GetChain(r.Context())
			if err != nil || !chain.ForgeBlockReceipts {
				// Forging disabled or no chain context, pass through
				next.ServeRPC(w, r)
				return
			}
			// we're in business
			var blockNumber [1]ethtypes.BlockNumber
			if err := json.Unmarshal(r.Params, &blockNumber); err != nil {
				_ = w.Send(nil, err)
				return
			}

			var rawLogs json.RawMessage
			var block json.RawMessage

			var wg errgroup.Group
			wg.Go(func() error {
				return jrpcutil.Do(r.Context(), next, &rawLogs, "eth_getLogs", []any{
					ethtypes.FilterQuery{
						FromBlock: &blockNumber[0],
						ToBlock:   &blockNumber[0],
					},
				})
			})
			wg.Go(func() error {
				return jrpcutil.Do(r.Context(), next, &block, "eth_getBlockByNumber", []any{
					blockNumber[0],
					true,
				})
			})

			if err := wg.Wait(); err != nil {
				_ = w.Send(nil, err)
				return
			}

			var transactions struct {
				Transactions []json.RawMessage `json:"transactions"`
			}
			if err := json.Unmarshal(block, &transactions); err != nil {
				_ = w.Send(nil, err)
				return
			}

			var logs []json.RawMessage
			if err := json.Unmarshal(rawLogs, &logs); err != nil {
				_ = w.Send(nil, err)
				return
			}

			var logDetails []struct {
				TransactionHash common.Hash `json:"transactionHash"`
			}
			if err := json.Unmarshal(rawLogs, &logDetails); err != nil {
				_ = w.Send(nil, err)
			}

			var wr jx.Writer
			wr.ArrStart()
			for i, transaction := range transactions.Transactions {
				if i != 0 {
					wr.Comma()
				}
				d := jx.DecodeBytes(transaction)
				obj, err := d.ObjIter()
				if err != nil {
					_ = w.Send(nil, err)
					return
				}
				var hash common.Hash
				wr.ObjStart()
				first := true
				for obj.Next() {
					switch {
					case bytes.Equal(obj.Key(), []byte("hash")):
						if first {
							first = false
						} else {
							wr.Comma()
						}
						wr.FieldStart("transactionHash")
						raw, err := d.Raw()
						if err != nil {
							_ = w.Send(nil, err)
							return
						}
						wr.Raw(raw)
						if err = json.Unmarshal(raw, &hash); err != nil {
							_ = w.Send(nil, err)
							return
						}
					case bytes.Equal(obj.Key(), []byte("blockHash")),
						bytes.Equal(obj.Key(), []byte("transactionIndex")),
						bytes.Equal(obj.Key(), []byte("to")),
						bytes.Equal(obj.Key(), []byte("from")),
						bytes.Equal(obj.Key(), []byte("type")),
						bytes.Equal(obj.Key(), []byte("blockNumber")):
						if first {
							first = false
						} else {
							wr.Comma()
						}
						wr.ByteStr(obj.Key())
						wr.RawStr(":")
						raw, err := d.Raw()
						if err != nil {
							_ = w.Send(nil, err)
							return
						}
						wr.Raw(raw)
					default:
						if err = d.Skip(); err != nil {
							_ = w.Send(nil, err)
							return
						}
					}
				}
				if !first {
					wr.Comma()
				}
				wr.FieldStart("logs")
				wr.ArrStart()
				first = true
				for j, log := range logs {
					if logDetails[j].TransactionHash != hash {
						continue
					}

					if first {
						first = false
					} else {
						wr.Comma()
					}

					wr.Raw(log)
				}
				wr.ArrEnd()
				wr.ObjEnd()
			}
			wr.ArrEnd()

			_ = w.Send(json.RawMessage(wr.Buf), nil)
		default:
			next.ServeRPC(w, r)
		}
	})
}
