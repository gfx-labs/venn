package vtrace

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gfx-labs/utilgo/fxplus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/fx"
)

// TraceProvider is a marker so handlers can depend on tracing being initialized.
type TraceProvider struct{}

type Params struct {
	fx.In

	Lc          fx.Lifecycle
	Log         *slog.Logger
	ServiceName fxplus.ComponentName
}

type Result struct {
	fx.Out

	Output *TraceProvider
}

func NewTraceProvider(p Params) (r Result, err error) {
	shutdown, err := InitTracing(context.Background(), Config{
		ServiceName: string(p.ServiceName),
		SampleRatio: SampleRatioFromEnv(DefaultSampleRatio),
		Logger:      p.Log,
	})
	if err != nil {
		p.Log.Warn("error initializing tracing", "err", err)
	} else {
		p.Lc.Append(fx.Hook{OnStop: shutdown})
	}
	r.Output = &TraceProvider{}
	return r, nil
}

// ExtractContext reads incoming trace headers into the request context without
// creating a span. The JSON-RPC middleware creates the only server span.
func ExtractContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
