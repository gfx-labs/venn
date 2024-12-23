package oracles

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/cel-go/cel"
)

func HttpJsonMap(r *http.Request) (map[string]any, error) {
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}
	var res map[string]any
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func CompileCel(celExpr string, opts ...cel.EnvOption) (cel.Program, map[string]any, error) {
	env, err := cel.NewEnv(
		append(
			[]cel.EnvOption{
				cel.Variable("time.now.unix_ms", cel.IntType),
				cel.Variable("time.now.unix", cel.IntType),
			},
			opts...)...,
	)
	if err != nil {
		return nil, nil, err
	}
	ast, issues := env.Compile(celExpr)
	if issues != nil && issues.Err() != nil {
		return nil, nil, issues.Err()
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, nil, fmt.Errorf("cel program construction error: %w", err)
	}
	now := time.Now()
	m := map[string]any{
		// NOTE: these are caddy placeholders, for future possible compatibility
		"time.now.unix_ms": now.UnixMilli(),
		"time.now.unix":    now.Unix(),
	}
	return prg, m, nil
}
