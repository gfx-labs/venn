package callcenter

import (
	"encoding/json"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
)

type InputData struct {
	remote Remote
}

func NewInputData(remote Remote) *InputData {
	return &InputData{
		remote: remote,
	}
}

func (T *InputData) ServeRPC(w jrpc.ResponseWriter, r *jrpc.Request) {
	switch r.Method {
	case "eth_call":
		var params []json.RawMessage
		if err := json.Unmarshal(r.Params, &params); err != nil {
			_ = w.Send(nil, jsonrpc.NewInvalidRequestError("params must be an array"))
			return
		}
		if len(params) != 2 {
			_ = w.Send(nil, jsonrpc.NewInvalidRequestError("expected 2 params"))
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
			T.remote.ServeRPC(w, r)
			return
		}

		if hasInput {
			call["data"] = input
		} else if hasData {
			call["input"] = data
		}

		var err error
		r.Params, err = json.Marshal([]any{call, params[1]})
		if err != nil {
			_ = w.Send(nil, jsonrpc.NewInternalError(err.Error()))
			return
		}

		T.remote.ServeRPC(w, r)
	default:
		T.remote.ServeRPC(w, r)
	}
}

var _ Remote = (*InputData)(nil)
