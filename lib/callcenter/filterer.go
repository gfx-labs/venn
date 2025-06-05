package callcenter

import (
	"gfx.cafe/open/jrpc"
)

// Filterer makes sure only certain requests can be made to a particular remote.
type Filterer struct {
	methods map[string]bool
}

func NewFilterer(methods map[string]bool) *Filterer {
	return &Filterer{
		methods: methods,
	}
}

func (T *Filterer) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		if ok, included := T.methods[r.Method]; included && !ok {
			_ = w.Send(nil, ErrMethodNotAllowed)
			return
		}
		next.ServeRPC(w, r)
	})
}

var _ Middleware = (*Filterer)(nil)
