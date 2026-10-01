package vtrace

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newTestProvider(ratio float64) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(NewErrorKeepingSampler(ratio)),
		sdktrace.WithSpanProcessor(NewErrorKeepingProcessor(sdktrace.NewSimpleSpanProcessor(exp))),
	)
	return tp, exp
}

func TestUnsampledSuccessIsDropped(t *testing.T) {
	tp, exp := newTestProvider(0)
	_, span := tp.Tracer("t").Start(context.Background(), "eth_call")
	span.End()
	if n := len(exp.GetSpans()); n != 0 {
		t.Fatalf("exported %d spans, want 0", n)
	}
}

func TestUnsampledErrorIsKept(t *testing.T) {
	tp, exp := newTestProvider(0)
	_, span := tp.Tracer("t").Start(context.Background(), "eth_call")
	span.RecordError(errors.New("boom"))
	span.SetStatus(codes.Error, "boom")
	span.End()
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported %d spans, want 1", len(spans))
	}
	if !spans[0].SpanContext.IsSampled() {
		t.Error("exported error span is not marked sampled")
	}
	if spans[0].Status.Code != codes.Error {
		t.Errorf("status = %v, want Error", spans[0].Status.Code)
	}
}

func TestRatioOneKeepsEverything(t *testing.T) {
	tp, exp := newTestProvider(1)
	for i := 0; i < 10; i++ {
		_, span := tp.Tracer("t").Start(context.Background(), "eth_call")
		span.End()
	}
	if n := len(exp.GetSpans()); n != 10 {
		t.Fatalf("exported %d spans, want 10", n)
	}
}

func TestSampledParentIsFollowed(t *testing.T) {
	tp, exp := newTestProvider(0)
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), parent)
	_, span := tp.Tracer("t").Start(ctx, "eth_call")
	span.End()
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported %d spans, want 1", len(spans))
	}
	if spans[0].Parent.SpanID() != parent.SpanID() {
		t.Error("span is not a child of the remote parent")
	}
}

func TestSampleRatioFromEnv(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want float64
	}{
		{"", 0.05},
		{"0.25", 0.25},
		{"1", 1},
		{"2", 0.05},
		{"-1", 0.05},
		{"nope", 0.05},
	} {
		t.Setenv(envSamplerArg, tc.val)
		if got := SampleRatioFromEnv(0.05); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.val, got, tc.want)
		}
	}
}

func TestInitTracingResource(t *testing.T) {
	// InitTracing changes process-wide state, so this test must not run in parallel.
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
	t.Setenv("HOSTNAME", "test-host")
	ctx := context.Background()
	exp := tracetest.NewInMemoryExporter()
	shutdown, err := InitTracing(ctx, Config{
		ServiceName: "venn",
		SampleRatio: 1,
		Exporter:    exp,
	})
	if err != nil {
		t.Fatalf("InitTracing: %v", err)
	}
	_, span := otel.Tracer("t").Start(ctx, "resource-test")
	span.End()
	// flush before reading; InMemoryExporter.Shutdown clears stored spans
	if err := otel.GetTracerProvider().(*sdktrace.TracerProvider).ForceFlush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	spans := exp.GetSpans()
	t.Cleanup(func() { _ = shutdown(ctx) })
	if len(spans) != 1 {
		t.Fatalf("exported %d spans, want 1", len(spans))
	}
	attrs := spans[0].Resource.Set()
	for key, want := range map[attribute.Key]string{
		"host.name":         "test-host",
		"service.name":      "venn",
		"service.namespace": DefaultServiceNamespace,
	} {
		got, ok := attrs.Value(key)
		if !ok || got.AsString() != want {
			t.Errorf("%s = %v, want %q", key, got, want)
		}
	}
}

func TestExtractContextCreatesNoSpan(t *testing.T) {
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })
	otel.SetTextMapPropagator(propagation.TraceContext{})

	var got trace.SpanContext
	h := ExtractContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = trace.SpanContextFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodPost, "/ethereum", nil)
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got.TraceID().String() != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace id = %s, want incoming trace id", got.TraceID())
	}
	if got.SpanID().String() != "b7ad6b7169203331" {
		t.Errorf("span id = %s, want incoming parent span id (no new span)", got.SpanID())
	}
	if !got.IsRemote() {
		t.Error("span context should be the remote parent")
	}
}
