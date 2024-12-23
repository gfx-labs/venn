package callcenter

import (
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
)

// Filterer makes sure only certain requests can be made to a particular remote.
type Filterer struct {
	methods map[string]bool

	remote Remote
}

func NewFilterer(remote Remote, methods map[string]bool) *Filterer {
	return &Filterer{
		methods: methods,
		remote:  remote,
	}
}

func (T *Filterer) ServeRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
	if ok, included := T.methods[r.Method]; included && !ok {
		_ = w.Send(nil, ErrMethodNotAllowed)
		return
	}

	T.remote.ServeRPC(w, r)
}

func (T *Filterer) Close() error {
	return T.remote.Close()
}

var _ Remote = (*Filterer)(nil)
