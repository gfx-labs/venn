package vtrace

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

const (
	envTracesEndpoint = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
	envEndpoint       = "OTEL_EXPORTER_OTLP_ENDPOINT"
	// standard otel env var, read as the head sampling ratio for new traces
	envSamplerArg = "OTEL_TRACES_SAMPLER_ARG"

	DefaultServiceNamespace = "venn"
	DefaultSampleRatio      = 0.05
)

type Config struct {
	ServiceName      string
	ServiceNamespace string
	// SampleRatio is the fraction of new traces exported. Errored spans are
	// always exported. Traces started upstream follow the caller's decision.
	SampleRatio float64
	// Exporter overrides the OTLP exporter, mainly for tests.
	Exporter sdktrace.SpanExporter
	Logger   *slog.Logger
}

type ShutdownFunc func(ctx context.Context) error

// SampleRatioFromEnv returns OTEL_TRACES_SAMPLER_ARG if it is a valid ratio, else def.
func SampleRatioFromEnv(def float64) float64 {
	v := os.Getenv(envSamplerArg)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || f > 1 {
		return def
	}
	return f
}

func newExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	if os.Getenv(envTracesEndpoint) != "" || os.Getenv(envEndpoint) != "" {
		return otlptracehttp.New(ctx)
	}
	// no endpoint configured, discard everything
	return stdouttrace.New(stdouttrace.WithWriter(io.Discard))
}

func newResource(cfg Config) (*resource.Resource, error) {
	hostName := os.Getenv("HOSTNAME")
	if hostName == "" {
		hostName = "<unknown>"
	}
	// Schemaless so it inherits the SDK resource schema. Environment is merged
	// last so OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES can override.
	r, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.HostName(hostName),
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceNamespace(cfg.ServiceNamespace),
		),
	)
	if err != nil {
		return nil, err
	}
	return resource.Merge(r, resource.Environment())
}

// InitTracing installs a global tracer provider and propagator.
func InitTracing(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	if cfg.ServiceNamespace == "" {
		cfg.ServiceNamespace = DefaultServiceNamespace
	}
	exporter := cfg.Exporter
	if exporter == nil {
		var err error
		exporter, err = newExporter(ctx)
		if err != nil {
			return nil, err
		}
	}
	r, err := newResource(cfg)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(r),
		sdktrace.WithSampler(NewErrorKeepingSampler(cfg.SampleRatio)),
		sdktrace.WithSpanProcessor(NewErrorKeepingProcessor(sdktrace.NewBatchSpanProcessor(exporter))),
	)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	otel.SetTracerProvider(tp)

	if cfg.Logger != nil {
		cfg.Logger.Info("tracing initialized",
			"service", cfg.ServiceName,
			"namespace", cfg.ServiceNamespace,
			"sample_ratio", cfg.SampleRatio)
	}
	return tp.Shutdown, nil
}
