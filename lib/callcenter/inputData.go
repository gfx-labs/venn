package callcenter

import (
	"encoding/json"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
)

type InputData struct {
}

func (T *InputData) Middleware(next jrpc.Handler) jrpc.Handler {
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		switch r.Method {
		case "eth_call":
			var params []json.RawMessage
			if err := json.Unmarshal(r.Params, &params); err != nil {
				_ = w.Send(nil, jsonrpc.NewInvalidRequestError("params must be an array"))
				return
			}
			if len(params) < 2 || len(params) > 3 {
				_ = w.Send(nil, jsonrpc.NewInvalidRequestError("expected 2-3 parameters"))
				return
			}

			var call map[string]json.RawMessage
			if err := json.Unmarshal(params[0], &call); err != nil {
				_ = w.Send(nil, jsonrpc.NewInvalidRequestError("expected first parameter to be an object"))
				return
			}

			input, hasInput := call["input"]
			data, hasData := call["data"]

			if (hasInput && hasData) || (!hasInput && !hasData) {
				next.ServeRPC(w, r)
				return
			}

			if hasInput {
				call["data"] = input
			} else if hasData {
				call["input"] = data
			}

			newParams := []any{call, params[1]}
			if len(params) == 3 {
				newParams = append(newParams, params[2])
			}

			var err error
			r.Params, err = json.Marshal(newParams)
			if err != nil {
				_ = w.Send(nil, jsonrpc.NewInternalError(err.Error()))
				return
			}

			next.ServeRPC(w, r)
		default:
			next.ServeRPC(w, r)
		}
	})
}
