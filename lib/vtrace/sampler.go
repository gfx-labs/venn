package vtrace

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// errorKeepingSampler makes the normal parent-based ratio decision, but
// records spans it would have dropped so the processor can still export them
// if they end in error.
type errorKeepingSampler struct {
	base sdktrace.Sampler
}

func NewErrorKeepingSampler(ratio float64) sdktrace.Sampler {
	return errorKeepingSampler{
		base: sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio)),
	}
}

func (s errorKeepingSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	res := s.base.ShouldSample(p)
	if res.Decision == sdktrace.Drop {
		res.Decision = sdktrace.RecordOnly
	}
	return res
}

func (s errorKeepingSampler) Description() string {
	return fmt.Sprintf("ErrorKeeping{%s}", s.base.Description())
}

// errorKeepingProcessor forwards sampled spans, and unsampled spans whose
// status is Error, to the next processor. Unsampled errors are marked sampled
// because the batch processor discards spans without the sampled flag.
type errorKeepingProcessor struct {
	next sdktrace.SpanProcessor
}

func NewErrorKeepingProcessor(next sdktrace.SpanProcessor) sdktrace.SpanProcessor {
	return errorKeepingProcessor{next: next}
}

func (p errorKeepingProcessor) OnStart(ctx context.Context, s sdktrace.ReadWriteSpan) {
	p.next.OnStart(ctx, s)
}

func (p errorKeepingProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
	if s.SpanContext().IsSampled() {
		p.next.OnEnd(s)
		return
	}
	if s.Status().Code == codes.Error {
		p.next.OnEnd(sampledSpan{ReadOnlySpan: s})
	}
}

func (p errorKeepingProcessor) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

func (p errorKeepingProcessor) ForceFlush(ctx context.Context) error {
	return p.next.ForceFlush(ctx)
}

type sampledSpan struct {
	sdktrace.ReadOnlySpan
}

func (s sampledSpan) SpanContext() trace.SpanContext {
	sc := s.ReadOnlySpan.SpanContext()
	return sc.WithTraceFlags(sc.TraceFlags().WithSampled(true))
}
