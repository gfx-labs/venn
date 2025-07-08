package jrpcutil

import (
	"context"
	"encoding/json"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
	"github.com/bytedance/sonic"

	"gfx.cafe/gfx/venn/lib/subctx"
)

func Do(ctx context.Context, handler jrpc.Handler, result any, method string, args any) error {
	var icept Interceptor
	r, err := jsonrpc.NewRequest(subctx.WithInternal(ctx, true), jsonrpc.NewNullIDPtr(), method, args)
	if err != nil {
		return err
	}
	handler.ServeRPC(&icept, r)
	if icept.Error != nil {
		return icept.Error
	}

	if res, ok := result.(*json.RawMessage); ok {
		switch iceptRes := icept.Result.(type) {
		case json.RawMessage:
			*res = iceptRes
			return nil
		case sonic.NoCopyRawMessage:
			*res = json.RawMessage(iceptRes)
			return nil
		}

		*res, err = sonic.Marshal(icept.Result)
		return err
	}

	var b json.RawMessage
	b, err = sonic.Marshal(icept.Result)
	if err != nil {
		return err
	}

	return sonic.Unmarshal(b, result)
}
