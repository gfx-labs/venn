package vtrace

import (
	"context"
	"errors"
	"testing"

	"github.com/gfx-labs/jrpc/pkg/jsonrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type nopWriter struct{}

func (nopWriter) Send(any, error) error            { return nil }
func (nopWriter) Notify(string, any) error         { return nil }
func (nopWriter) ExtraFields() jsonrpc.ExtraFields { return nil }

func serve(t *testing.T, ratio float64, h jsonrpc.HandlerFunc) []sdktrace.ReadOnlySpan {
	t.Helper()
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	exp := tracetest.NewInMemoryExporter()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(
		sdktrace.WithSampler(NewErrorKeepingSampler(ratio)),
		sdktrace.WithSpanProcessor(NewErrorKeepingProcessor(sdktrace.NewSimpleSpanProcessor(exp))),
	))
	req, err := jsonrpc.NewRequest(context.Background(), nil, "eth_call", []any{map[string]string{"data": "0xdeadbeef"}, "latest"})
	if err != nil {
		t.Fatal(err)
	}
	mw := RPCMiddleware(func(*jsonrpc.Request) []attribute.KeyValue {
		return []attribute.KeyValue{attribute.String("chain", "ethereum")}
	})
	mw(h).ServeRPC(nopWriter{}, req)
	return exp.GetSpans().Snapshots()
}

func TestRPCMiddlewareRecordsErrors(t *testing.T) {
	spans := serve(t, 0, func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		_ = w.Send(nil, errors.New("execution reverted"))
	})
	if len(spans) != 1 {
		t.Fatalf("exported %d spans, want 1 (errors must bypass sampling)", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Errorf("status = %v, want Error", spans[0].Status().Code)
	}
}

func TestRPCMiddlewareAttributes(t *testing.T) {
	spans := serve(t, 1, func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		_ = w.Send("0x1", nil)
	})
	if len(spans) != 1 {
		t.Fatalf("exported %d spans, want 1", len(spans))
	}
	s := spans[0]
	if s.Name() != "eth_call" {
		t.Errorf("name = %q, want eth_call", s.Name())
	}
	if s.Status().Code == codes.Error {
		t.Error("successful request marked as error")
	}
	got := map[attribute.Key]string{}
	for _, kv := range s.Attributes() {
		got[kv.Key] = kv.Value.Emit()
	}
	if _, ok := got["params"]; ok {
		t.Error("params attribute must not be recorded")
	}
	if got["method"] != "eth_call" || got["chain"] != "ethereum" {
		t.Errorf("attributes = %v, want method and chain", got)
	}
}

func TestRPCMiddlewareSuccessUnsampledDropped(t *testing.T) {
	spans := serve(t, 0, func(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
		_ = w.Send("0x1", nil)
	})
	if len(spans) != 0 {
		t.Fatalf("exported %d spans, want 0", len(spans))
	}
}
