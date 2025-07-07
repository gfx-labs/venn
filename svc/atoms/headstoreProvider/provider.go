package headstoreProvider

import (
	"gfx.cafe/gfx/venn/lib/stores/headstore"
	"gfx.cafe/gfx/venn/svc/stores/headstores/redihead"
	"go.uber.org/fx"
)

type Params struct {
	fx.In

	Redihead *redihead.Redihead `optional:"true"`
}

type Result struct {
	fx.Out

	Headstore headstore.Store
}

// New creates a headstore instance that can be used by both vennstore and cluster
func New(p Params) (r Result, err error) {
	if p.Redihead != nil {
		r.Headstore = p.Redihead
	} else {
		r.Headstore = headstore.NewAtomic()
	}
	return
}