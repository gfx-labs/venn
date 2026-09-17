package callcenter

import (
	"gfx.cafe/open/jrpc"
)

// Filterer makes sure only certain requests can be made to a particular remote.
type Filterer struct {
	methods   map[string]bool
	whitelist map[string]bool
}

func NewFilterer(methods map[string]bool) *Filterer {
	return &Filterer{
		methods: methods,
	}
}

// NewFiltererWithWhitelist additionally requires a method to be enabled in the
// whitelist. A nil whitelist preserves legacy behavior; an empty one denies all.
func NewFiltererWithWhitelist(methods, whitelist map[string]bool) *Filterer {
	return &Filterer{methods: methods, whitelist: whitelist}
}

func (T *Filterer) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		if T.whitelist != nil && !T.whitelist[r.Method] {
			_ = w.Send(nil, ErrMethodNotAllowed)
			return
		}
		if ok, included := T.methods[r.Method]; included && !ok {
			_ = w.Send(nil, ErrMethodNotAllowed)
			return
		}
		next.ServeRPC(w, r)
	})
}

var _ Middleware = (*Filterer)(nil)
