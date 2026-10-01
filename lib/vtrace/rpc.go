package vtrace

import (
	"fmt"

	"github.com/gfx-labs/jrpc"
	"github.com/gfx-labs/jrpc/contrib/jrpcutil"
	"github.com/gfx-labs/jrpc/pkg/jsonrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RPCMiddleware starts one server span per JSON-RPC request, named after the
// method. Request params are deliberately not recorded: they are large and
// were the bulk of venn's trace volume. Errors sent to the client mark the
// span as Error so the sampler keeps it.
func RPCMiddleware(attrs func(req *jsonrpc.Request) []attribute.KeyValue) func(jrpc.Handler) jrpc.Handler {
	return func(next jrpc.Handler) jrpc.Handler {
		tracer := otel.Tracer("jrpc")
		return jrpc.HandlerFunc(func(w jsonrpc.ResponseWriter, req *jsonrpc.Request) {
			kv := []attribute.KeyValue{attribute.String("method", req.Method)}
			if attrs != nil {
				kv = append(kv, attrs(req)...)
			}
			ctx, span := tracer.Start(req.Context(), req.Method,
				trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(kv...))
			defer span.End()

			ew := &jrpcutil.ErrorRecorder{ResponseWriter: w}
			next.ServeRPC(ew, req.WithContext(ctx))

			if err := ew.Error(); err != nil {
				span.SetStatus(codes.Error, fmt.Sprintf("error: %s", err))
				span.RecordError(err)
			}
		})
	}
}
