package jrpcutil

import (
	"errors"

	"gfx.cafe/open/jrpc"
)

type Interceptor struct {
	Result any
	Error  error
}

func (T *Interceptor) Notify(_ string, _ any) error {
	return errors.New("not supported")
}

func (T *Interceptor) Send(v any, err error) error {
	T.Result, T.Error = v, err
	return nil
}

var _ jrpc.ResponseWriter = (*Interceptor)(nil)
