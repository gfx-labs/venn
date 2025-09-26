package jrpcutil

import (
	"errors"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
)

type Interceptor struct {
	Result any
	Error  error
	extraFields jsonrpc.ExtraFields
}

func (T *Interceptor) Notify(_ string, _ any) error {
	return errors.New("not supported")
}

func (T *Interceptor) Send(v any, err error) error {
	T.Result, T.Error = v, err
	return nil
}

func (T *Interceptor) ExtraFields() jsonrpc.ExtraFields {
	if T.extraFields == nil {
		T.extraFields = make(jsonrpc.ExtraFields)
	}
	return T.extraFields
}

var _ jrpc.ResponseWriter = (*Interceptor)(nil)
