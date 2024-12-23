package blockoracle

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"gfx.cafe/gfx/venn/lib/oracles"
	"github.com/google/cel-go/cel"
)

// blockoracles are used to try to figure out the current head block

type BlockOracle = oracles.Oracle[int]
type BlockOracleFunc = oracles.OracleFunc[int]

func HttpJsonOracle(urlString string, celExpr string) (BlockOracle, error) {
	prg, evalCtx, err := oracles.CompileCel(celExpr, cel.Variable("body", cel.MapType(cel.StringType, cel.DynType)))
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}
	// TODO: maybe allow more than GET requests, but this should be enough for 99% of apis.
	r, err := http.NewRequest("GET", parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	return BlockOracleFunc(func(ctx context.Context) (int, error) {
		res, err := oracles.HttpJsonMap(r.WithContext(ctx))
		if err != nil {
			return 0, err
		}
		evalCtx["body"] = res
		val, _, err := prg.ContextEval(ctx, evalCtx)
		if err != nil {
			return 0, err
		}
		if val.Type() != cel.IntType {
			return 0, fmt.Errorf("cel return type %v, want %v", val.Type(), cel.IntType)
		}
		bres, ok := val.Value().(int64)
		if !ok {
			return 0, fmt.Errorf("expected go type %s", "int64")
		}
		return int(bres), nil
	}), nil
}
