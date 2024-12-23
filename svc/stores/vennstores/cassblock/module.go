package cassblock

/*
import (
	"context"
	"log/slog"

	"gfx.cafe/gfx/venn/lib/vennstore"
	"gfx.cafe/gfx/venn/svc/cass"
	"github.com/scylladb/gocqlx/v2/qb"
	"go.uber.org/fx"
)

var _ vennstore.Store = (*Cassblock)(nil)

type Params struct {
	fx.In

	Log  *slog.Logger
	Cass *cass.Scylla `optional:"true"`
}

type Result struct {
	fx.Out

	Cassblock *Cassblock `optional:"true"`
}

func New(params Params) (r Result, err error) {
	if params.Cass == nil {
		params.Log.Warn("cassblock disabled", "reason", "no cassandra")
		return
	}
	r.Cassblock = &Cassblock{
		cass: params.Cass,
		log:  params.Log,
	}
	// run db migrations
	err = vennstore.Up(r.Cassblock.log, r.Cassblock.cass.C())
	if err != nil {
		return
	}
	return
}

type Cassblock struct {
	cass *cass.Scylla
	log  *slog.Logger
}

func (b *Cassblock) GetRawBlock(ctx context.Context, query *vennstore.RawBlockQuery) (*vennstore.RawBlock, error) {
	if query.Hash != nil {
		hash := *query.Hash
		raw := &vennstore.RawBlock{}
		err := b.cass.C().Query(vennstore.RawBlocksTable.Select()).BindMap(qb.M{"chain": query.Chain, "hash": hash[:]}).SelectRelease(raw)
		if err != nil {
			return nil, err
		}
		return raw, nil
	}
	number := 0
	if query.Number != nil {
		number = *query.Number
	}
	raw := &vennstore.RawBlock{}
	err := b.cass.C().Query(vennstore.RawBlocksTable.Select()).BindMap(qb.M{"chain": query.Chain, "number": number}).SelectRelease(raw)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (b *Cassblock) PutRawBlock(ctx context.Context, rb *vennstore.RawBlock) (bool, error) {
	err := vennstore.RawBlocksTable.InsertQueryContext(ctx, *b.cass.C()).BindStruct(b).ExecRelease()
	if err != nil {
		return false, err
	}
	return false, nil
}

*/
