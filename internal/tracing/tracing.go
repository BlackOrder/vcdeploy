// Package tracing provides OpenTelemetry tracing instrumentation for vcdeploy.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds tracing configuration.
type Config struct {
	// Enabled controls whether tracing is active.
	Enabled bool `yaml:"enabled"`
	// Endpoint is the OTLP collector endpoint (e.g., "localhost:4317").
	Endpoint string `yaml:"endpoint"`
	// ServiceName is the name used in traces.
	ServiceName string `yaml:"service_name"`
	// SampleRate is the sampling rate (0.0-1.0). 1.0 = always sample.
	SampleRate float64 `yaml:"sample_rate"`
	// Insecure disables TLS for the OTLP connection.
	Insecure bool `yaml:"insecure"`
}

// DefaultConfig returns a default tracing configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		Endpoint:    "localhost:4317",
		ServiceName: "vcdeploy",
		SampleRate:  0.1,
		Insecure:    true,
	}
}

// version is set at build time via ldflags.
var version = "dev"

// SetVersion sets the version for trace resource attributes.
func SetVersion(v string) {
	version = v
}

// InitTracer initializes the OpenTelemetry tracer provider.
// Returns a shutdown function that should be called on application exit.
func InitTracer(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !cfg.Enabled {
		// Return a no-op shutdown function when tracing is disabled.
		return func(context.Context) error { return nil }, nil
	}

	// Set up OTLP exporter options.
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	// Create the OTLP exporter.
	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	// Build the resource with service information.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating trace resource: %w", err)
	}

	// Configure the sampler based on sample rate.
	var sampler sdktrace.Sampler
	switch {
	case cfg.SampleRate >= 1.0:
		sampler = sdktrace.AlwaysSample()
	case cfg.SampleRate <= 0:
		sampler = sdktrace.NeverSample()
	default:
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create the tracer provider.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Register as the global tracer provider.
	otel.SetTracerProvider(tp)

	// Set up propagators for distributed tracing.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// Tracer returns a named tracer for instrumentation.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// SpanFromContext returns the current span from the context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// AddSpanAttributes adds attributes to the current span in the context.
func AddSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// RecordError records an error on the current span.
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() && err != nil {
		span.RecordError(err)
	}
}

// StartSpan starts a new span with the given name.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer("vcdeploy").Start(ctx, name, opts...)
}

// SpanKind constants for common span types.
var (
	SpanKindServer   = trace.WithSpanKind(trace.SpanKindServer)
	SpanKindClient   = trace.WithSpanKind(trace.SpanKindClient)
	SpanKindInternal = trace.WithSpanKind(trace.SpanKindInternal)
)

// Common attribute keys for vcdeploy.
var (
	AttrDeploymentID = attribute.Key("vcdeploy.deployment.id")
	AttrProjectName  = attribute.Key("vcdeploy.project.name")
	AttrAgentID      = attribute.Key("vcdeploy.agent.id")
	AttrUserID       = attribute.Key("vcdeploy.user.id")
	AttrOperation    = attribute.Key("vcdeploy.operation")
)
