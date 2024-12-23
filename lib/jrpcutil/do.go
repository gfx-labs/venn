package jrpcutil

import (
	"context"
	"encoding/json"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"

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
		if iceptRes, ok := icept.Result.(json.RawMessage); ok {
			*res = iceptRes
			return nil
		}

		*res, err = json.Marshal(icept.Result)
		return err
	}

	var b json.RawMessage
	b, err = json.Marshal(icept.Result)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, result)
}
