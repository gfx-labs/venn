package callcenter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-faster/jx"

	"gfx.cafe/gfx/venn/lib/ethtypes"
	"gfx.cafe/gfx/venn/lib/jrpcutil"
)

// Validator validates that the result returned by the rpc is valid.
type Validator struct {
	remote Remote
	old    time.Duration

	head    hexutil.Uint64
	updated time.Time
	mu      sync.Mutex
}

func NewValidator(remote Remote, old time.Duration) *Validator {
	return &Validator{
		remote: remote,
		old:    old,
	}
}

func (T *Validator) updateHead(head hexutil.Uint64, updated time.Time) error {
	T.mu.Lock()
	defer T.mu.Unlock()

	if head < T.head {
		return fmt.Errorf("%w: %d -> %d", ErrHeadJumpedBackwards, T.head, head)
	}

	if head > T.head {
		T.head = head
		T.updated = updated
		return nil
	}

	if updated.Before(T.updated) {
		T.updated = updated
	}

	if time.Now().Sub(T.updated) > T.old {
		return ErrHeadOld
	}

	return nil
}

func (*Validator) isQueryingEthGetBlockByNumberWithLatest(params json.RawMessage) bool {
	d := jx.DecodeBytes(params)
	arr, err := d.ArrIter()
	if err != nil {
		return false
	}

	if !arr.Next() {
		return false
	}

	block, err := d.StrBytes()
	if err != nil {
		return false
	}

	return bytes.Equal(block, []byte("latest"))
}

func (*Validator) extractBlockTimeAndTimestamp(block any) (hexutil.Uint64, time.Time, error) {
	switch b := block.(type) {
	case ethtypes.BlockHeader:
		return b.Number, time.Unix(int64(b.Timestamp), 0), nil
	case ethtypes.TruncatedBlockHeader:
		return b.Number, time.Unix(int64(b.Timestamp), 0), nil
	case *ethtypes.BlockHeader:
		return b.Number, time.Unix(int64(b.Timestamp), 0), nil
	case *ethtypes.TruncatedBlockHeader:
		return b.Number, time.Unix(int64(b.Timestamp), 0), nil
	case json.RawMessage:
		if string(b) == "null" {
			return 0, time.Time{}, ErrHeadOld
		}
		d := jx.DecodeBytes(b)
		obj, err := d.ObjIter()
		if err != nil {
			return 0, time.Time{}, err
		}

		var number hexutil.Uint64
		var timestamp time.Time
		for obj.Next() {
			if bytes.Equal(obj.Key(), []byte("number")) {
				v, err := d.Raw()
				if err != nil {
					return 0, time.Time{}, err
				}
				if err = json.Unmarshal(v, &number); err != nil {
					return 0, time.Time{}, err
				}
			} else if bytes.Equal(obj.Key(), []byte("timestamp")) {
				v, err := d.Raw()
				if err != nil {
					return 0, time.Time{}, err
				}
				var ts hexutil.Uint64
				if err = json.Unmarshal(v, &ts); err != nil {
					return 0, time.Time{}, err
				}
				timestamp = time.Unix(int64(ts), 0)
			} else {
				if err = d.Skip(); err != nil {
					return 0, time.Time{}, err
				}
			}
		}

		return number, timestamp, nil
	default:
		return 0, time.Time{}, errors.New("expected block")
	}
}

type errTooSlow struct{}

func (errTooSlow) Error() string {
	return "too slow :("
}

func (T *Validator) ServeRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
	if r.Params != nil && !json.Valid(r.Params) {
		_ = w.Send(nil, jsonrpc.NewInvalidParamsError("params must be valid json"))
		return
	}

	switch r.Method {
	case "eth_getBlockByNumber":
		isLatest := T.isQueryingEthGetBlockByNumberWithLatest(r.Params)

		if isLatest {
			ctx, cancel := context.WithTimeoutCause(r.Context(), 15*time.Second, errTooSlow{})
			defer cancel()
			r = r.WithContext(ctx)
		}

		var icept jrpcutil.Interceptor
		T.remote.ServeRPC(&icept, r)

		// check if r.Params[0] == "latest"
		if icept.Error == nil && isLatest {
			// get block timestamp
			blockNumber, timestamp, err := T.extractBlockTimeAndTimestamp(icept.Result)
			if err != nil {
				icept.Error = err
			} else {
				icept.Error = T.updateHead(blockNumber, timestamp)
			}
		}

		if icept.Error == nil {
			// possibly an old head
			switch res := icept.Result.(type) {
			case json.RawMessage:
				if bytes.Equal(res, []byte("null")) {
					icept.Error = ErrHeadOld
				}
			case nil:
				icept.Error = ErrHeadOld
			}
		}

		if errors.Is(icept.Error, context.DeadlineExceeded) && errors.Is(context.Cause(r.Context()), errTooSlow{}) {
			icept.Error = ErrHeadOld
		}

		_ = w.Send(icept.Result, icept.Error)
	case "eth_blockNumber":
		var block json.RawMessage
		if err := jrpcutil.Do(r.Context(), T.remote, &block, "eth_getBlockByNumber", []any{"latest", false}); err != nil {
			_ = w.Send(nil, err)
			return
		}

		// get block timestamp
		blockNumber, timestamp, err := T.extractBlockTimeAndTimestamp(block)
		if err != nil {
			_ = w.Send(nil, err)
			return
		} else {
			err = T.updateHead(blockNumber, timestamp)
		}

		_ = w.Send(blockNumber, err)
	case "eth_getBlockByHash", "eth_getLogs", "eth_getBlockReceipts":
		var icept jrpcutil.Interceptor
		T.remote.ServeRPC(&icept, r)

		if icept.Error == nil {
			// possibly an old head
			switch res := icept.Result.(type) {
			case json.RawMessage:
				if bytes.Equal(res, []byte("null")) {
					_ = w.Send(icept.Result, ErrHeadOld)
					return
				}
			case nil:
				_ = w.Send(icept.Result, ErrHeadOld)
				return
			}
		}

		_ = w.Send(icept.Result, icept.Error)
	default:
		T.remote.ServeRPC(w, r)
	}
}

var _ Remote = (*Validator)(nil)
