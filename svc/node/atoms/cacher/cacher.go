package cacher

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"gfx.cafe/gfx/venn/lib/subctx"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-faster/jx"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/ethtypes"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/stores/blockstore"
	"gfx.cafe/gfx/venn/svc/node/quarks/cluster"
)

type Cacher struct {
	store blockstore.Store
}

type Params struct {
	fx.In

	Chains   map[string]*config.Chain
	Clusters *cluster.Clusters
	Blocks   blockstore.Store
}

type Result struct {
	fx.Out

	Cacher *Cacher
}

// maxLogRange is the max range for logs before they are ignored from the cache
const maxLogRange = 10

func New(p Params) (r Result, err error) {
	r.Cacher = &Cacher{
		store: p.Blocks,
	}
	return
}

func removeTransactionDetails(block json.RawMessage) (json.RawMessage, error) {
	d := jx.DecodeBytes(block)
	var w jx.Writer

	header, err := d.ObjIter()
	if err != nil {
		return nil, err
	}
	w.ObjStart()
	firstKey := true
	for header.Next() {
		if firstKey {
			firstKey = false
		} else {
			w.Comma()
		}
		w.ByteStr(header.Key())
		w.RawStr(":")
		if bytes.Equal(header.Key(), []byte("transactions")) {
			transactions, err := d.ArrIter()
			if err != nil {
				return nil, err
			}

			w.ArrStart()
			firstTransaction := true
			for transactions.Next() {
				transaction, err := d.ObjIter()
				if err != nil {
					return nil, err
				}

				for transaction.Next() {
					if bytes.Equal(transaction.Key(), []byte("hash")) {
						hash, err := d.Raw()
						if err != nil {
							return nil, err
						}
						if firstTransaction {
							firstTransaction = false
						} else {
							w.Comma()
						}
						w.Raw(hash)
					} else {
						if err = d.Skip(); err != nil {
							return nil, err
						}
					}
				}
			}
			w.ArrEnd()
		} else {
			raw, err := d.Raw()
			if err != nil {
				return nil, err
			}
			w.Raw(raw)
		}
	}
	w.ObjEnd()

	return w.Buf, nil
}

func rawStrToAddress(rawStr []byte) common.Address {
	if bytes.HasPrefix(rawStr, []byte("0x")) {
		rawStr = rawStr[2:]
	}

	var res common.Address
	if _, err := hex.Decode(res[:], rawStr); err != nil {
		res = common.Address{}
	}

	return res
}

func rawStrToHash(rawStr []byte) common.Hash {
	if bytes.HasPrefix(rawStr, []byte("0x")) {
		rawStr = rawStr[2:]
	}

	var res common.Hash
	if _, err := hex.Decode(res[:], rawStr); err != nil {
		res = common.Hash{}
	}

	return res
}

func (T *Cacher) filterLogs(filter *ethtypes.CompiledFilter, entries []*blockstore.Entry) (json.RawMessage, error) {
	var header ethtypes.LogAddressTopics

	var w jx.Writer
	w.ArrStart()
	firstLog := true
	for _, entry := range entries {
		d := jx.DecodeBytes(entry.Value)
		arr, err := d.ArrIter()
		if err != nil {
			return nil, err
		}
		for arr.Next() {
			raw, err := d.Raw()
			if err != nil {
				return nil, err
			}

			header.Topics = header.Topics[:0]
			header.Address = common.Address{}

			dd := jx.DecodeBytes(raw)

			log, err := dd.ObjIter()
			if err != nil {
				return nil, err
			}

			for log.Next() {
				if bytes.Equal(log.Key(), []byte("address")) {
					address, err := dd.StrBytes()
					if err != nil {
						return nil, err
					}

					header.Address = rawStrToAddress(address)
				} else if bytes.Equal(log.Key(), []byte("topics")) {
					topics, err := dd.ArrIter()
					if err != nil {
						return nil, err
					}

					for topics.Next() {
						topic, err := dd.StrBytes()
						if err != nil {
							return nil, err
						}

						header.Topics = append(header.Topics, rawStrToHash(topic))
					}
				} else {
					if err = dd.Skip(); err != nil {
						return nil, err
					}
				}
			}

			if !filter.Check(header) {
				continue
			}

			if firstLog {
				firstLog = false
			} else {
				w.Comma()
			}
			w.Raw(raw)
		}
	}
	w.ArrEnd()

	return w.Buf, nil
}

func (T *Cacher) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		chain, err := subctx.GetChain(r.Context())
		if err != nil {
			_ = w.Send(nil, err)
			return
		}
		switch r.Method {
		case "eth_getBlockByNumber":
			var params []json.RawMessage
			if err := json.Unmarshal(r.Params, &params); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(params) != 2 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 2 params"))
				return
			}

			var blockNumber ethtypes.BlockNumber
			if err := json.Unmarshal(params[0], &blockNumber); err != nil {
				_ = w.Send(nil, err)
				return
			}

			var details bool
			if err := json.Unmarshal(params[1], &details); err != nil {
				_ = w.Send(nil, err)
				return
			}

			if blockNumber >= 0 {
				// positive number, try to get from the store
				entries, err := T.store.Get(r.Context(), chain, blockstore.EntryBlockHeader, blockstore.QueryNumber(hexutil.Uint64(blockNumber)))
				if err != nil {
					_ = w.Send(nil, err)
					return
				}

				if len(entries) != 1 {
					_ = w.Send(nil, nil)
					return
				}

				if !details {
					ans, err := removeTransactionDetails(entries[0].Value)
					if err != nil {
						err = fmt.Errorf("failed removing transaction details: %w", err)
					}
					_ = w.Send(ans, err)
					return
				}

				_ = w.Send(entries[0].Value, nil)
				return
			}

			// otherwise, proxy back to the remote
			var block json.RawMessage
			if err := jrpcutil.Do(r.Context(), next, &block, "eth_getBlockByNumber", []any{blockNumber, true}); err != nil {
				_ = w.Send(nil, fmt.Errorf("getting block from remote: %w", err))
				return
			}

			var head struct {
				BlockNumber hexutil.Uint64 `json:"number"`
				BlockHash   common.Hash    `json:"hash"`
				ParentHash  common.Hash    `json:"parentHash"`
			}
			if err := json.Unmarshal(block, &head); err != nil {
				_ = w.Send(nil, err)
				return
			}

			if err := T.store.Put(r.Context(), chain, blockstore.EntryBlockHeader, &blockstore.Entry{
				BlockHash:   head.BlockHash,
				BlockNumber: head.BlockNumber,
				ParentHash:  &head.ParentHash,
				Value:       block,
			}); err != nil {
				_ = w.Send(nil, err)
				return
			}

			if !details && !bytes.Equal(block, []byte("null")) {
				raw, err := removeTransactionDetails(block)
				if err != nil {
					err = fmt.Errorf("failed to remove transaction details: %w", err)
				}
				_ = w.Send(raw, err)
				return
			}

			_ = w.Send(block, nil)
			return
		case "eth_getBlockByHash":
			var params []json.RawMessage
			if err := json.Unmarshal(r.Params, &params); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(params) != 2 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 2 params"))
				return
			}

			var blockHash common.Hash
			if err := json.Unmarshal(params[0], &blockHash); err != nil {
				_ = w.Send(nil, err)
				return
			}

			var details bool
			if err := json.Unmarshal(params[1], &details); err != nil {
				_ = w.Send(nil, err)
				return
			}

			entries, err := T.store.Get(r.Context(), chain, blockstore.EntryBlockHeader, blockstore.QueryHash(blockHash))
			if err != nil {
				_ = w.Send(nil, err)
				return
			}

			if len(entries) != 1 {
				_ = w.Send(nil, nil)
				return
			}

			if !details {
				raw, err := removeTransactionDetails(entries[0].Value)
				if err != nil {
					err = fmt.Errorf("failed to remove transaction details: %w", err)
				}
				_ = w.Send(raw, err)
				return
			}

			_ = w.Send(entries[0].Value, nil)
			return
		case "eth_getBlockReceipts":
			var params []ethtypes.BlockNumber
			if err := json.Unmarshal(r.Params, &params); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(params) != 1 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 1 parameter"))
				return
			}

			if params[0] >= 0 {
				entries, err := T.store.Get(r.Context(), chain, blockstore.EntryReceipts, blockstore.QueryNumber(hexutil.Uint64(params[0])))
				if err != nil {
					_ = w.Send(nil, err)
					return
				}

				if len(entries) != 1 {
					_ = w.Send(nil, nil)
					return
				}

				_ = w.Send(entries[0].Value, nil)
				return
			}

			// just ignore for now. it is a huge pain in the ass to do this and if using stalker it doesn't matter
			next.ServeRPC(w, r)
		case "eth_getLogs":
			var params []ethtypes.FilterQuery
			if err := json.Unmarshal(r.Params, &params); err != nil {
				_ = w.Send(nil, err)
				return
			}
			if len(params) != 1 {
				_ = w.Send(nil, jsonrpc.NewInvalidParamsError("expected 1 parameter"))
				return
			}

			var logs []*blockstore.Entry
			var err error
			if params[0].BlockHash != nil {
				logs, err = T.store.Get(r.Context(), chain, blockstore.EntryLogs, blockstore.QueryHash(*params[0].BlockHash))
			} else if params[0].FromBlock != nil && params[0].ToBlock != nil {
				if *params[0].ToBlock-*params[0].FromBlock > maxLogRange {
					next.ServeRPC(w, r)
					return
				}

				logs, err = T.store.Get(r.Context(), chain, blockstore.EntryLogs, blockstore.QueryRange{
					Start: hexutil.Uint64(*params[0].FromBlock),
					End:   hexutil.Uint64(*params[0].ToBlock),
				})
			} else {
				next.ServeRPC(w, r)
				return
			}
			if err != nil {
				_ = w.Send(nil, err)
				return
			}

			filter := ethtypes.CompileFilter(params[0].Addresses, params[0].Topics)
			raw, err := T.filterLogs(filter, logs)
			if err != nil {
				err = fmt.Errorf("failed to filter logs: %w", err)
			}
			_ = w.Send(raw, err)
			return
		default:
			next.ServeRPC(w, r)
		}
	})
}
