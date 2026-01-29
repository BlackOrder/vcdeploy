package tracing

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("default config should have Enabled=false")
	}
	if cfg.Endpoint != "localhost:4317" {
		t.Errorf("Endpoint = %s, want localhost:4317", cfg.Endpoint)
	}
	if cfg.ServiceName != "vcdeploy" {
		t.Errorf("ServiceName = %s, want vcdeploy", cfg.ServiceName)
	}
	if cfg.SampleRate != 0.1 {
		t.Errorf("SampleRate = %f, want 0.1", cfg.SampleRate)
	}
	if !cfg.Insecure {
		t.Error("default config should have Insecure=true")
	}
}

func TestSetVersion(t *testing.T) {
	original := version
	defer func() { version = original }()

	SetVersion("1.2.3")
	if version != "1.2.3" {
		t.Errorf("version = %s, want 1.2.3", version)
	}
}

func TestInitTracer_Disabled(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	ctx := context.Background()
	shutdown, err := InitTracer(ctx, cfg)
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}

	// Shutdown should be a no-op and succeed
	if err := shutdown(ctx); err != nil {
		t.Errorf("shutdown failed: %v", err)
	}
}

func TestTracer(t *testing.T) {
	tracer := Tracer("test-tracer")
	if tracer == nil {
		t.Fatal("Tracer returned nil")
	}

	// The tracer should be usable even without initialization
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if ctx == nil {
		t.Error("context from Tracer.Start is nil")
	}
}

func TestSpanFromContext(t *testing.T) {
	ctx := context.Background()

	// Without a span, should return a no-op span
	span := SpanFromContext(ctx)
	if span == nil {
		t.Fatal("SpanFromContext returned nil")
	}

	// With a span in context
	tracer := Tracer("test")
	ctx, newSpan := tracer.Start(ctx, "test-span")
	defer newSpan.End()

	retrieved := SpanFromContext(ctx)
	// Compare span contexts instead of spans directly (spans may not be comparable)
	if retrieved.SpanContext().TraceID() != newSpan.SpanContext().TraceID() {
		t.Error("SpanFromContext did not return a span with matching trace ID")
	}
}

func TestAddSpanAttributes(t *testing.T) {
	ctx := context.Background()

	// Should not panic with no span
	AddSpanAttributes(ctx, attribute.String("key", "value"))

	// With a span in context
	tracer := Tracer("test")
	ctx, span := tracer.Start(ctx, "test-span")
	defer span.End()

	// Should not panic
	AddSpanAttributes(ctx,
		attribute.String("key1", "value1"),
		attribute.Int("key2", 42),
	)
}

func TestRecordError(t *testing.T) {
	ctx := context.Background()

	// Should not panic with no span
	RecordError(ctx, errors.New("test error"))

	// Should not panic with nil error
	RecordError(ctx, nil)

	// With a span in context
	tracer := Tracer("test")
	ctx, span := tracer.Start(ctx, "test-span")
	defer span.End()

	// Should not panic
	RecordError(ctx, errors.New("test error in span"))
}

func TestStartSpan(t *testing.T) {
	ctx := context.Background()

	newCtx, span := StartSpan(ctx, "test-operation")
	if newCtx == nil {
		t.Error("StartSpan returned nil context")
	}
	if span == nil {
		t.Fatal("StartSpan returned nil span")
	}
	defer span.End()

	// Verify the span is in the context by comparing span contexts
	retrieved := SpanFromContext(newCtx)
	if retrieved.SpanContext().SpanID() != span.SpanContext().SpanID() {
		t.Error("span not correctly added to context")
	}
}

func TestStartSpanWithOptions(t *testing.T) {
	ctx := context.Background()

	// Test with SpanKind options
	testCases := []struct {
		name string
		opt  trace.SpanStartOption
	}{
		{"server", SpanKindServer},
		{"client", SpanKindClient},
		{"internal", SpanKindInternal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, span := StartSpan(ctx, "test-"+tc.name, tc.opt)
			if span == nil {
				t.Error("StartSpan returned nil span")
			}
			span.End()
		})
	}
}

func TestAttributeKeys(t *testing.T) {
	// Verify attribute keys are defined correctly
	tests := []struct {
		key      attribute.Key
		expected string
	}{
		{AttrDeploymentID, "vcdeploy.deployment.id"},
		{AttrProjectName, "vcdeploy.project.name"},
		{AttrAgentID, "vcdeploy.agent.id"},
		{AttrUserID, "vcdeploy.user.id"},
		{AttrOperation, "vcdeploy.operation"},
	}

	for _, tt := range tests {
		if string(tt.key) != tt.expected {
			t.Errorf("attribute key %v = %s, want %s", tt.key, tt.key, tt.expected)
		}
	}
}

func TestAttributeKeyUsage(t *testing.T) {
	// Test that attribute keys can be used to create attributes
	attrs := []attribute.KeyValue{
		AttrDeploymentID.String("deploy-123"),
		AttrProjectName.String("my-project"),
		AttrAgentID.String("agent-1"),
		AttrUserID.Int64(42),
		AttrOperation.String("deploy"),
	}

	if len(attrs) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(attrs))
	}

	// Verify values
	if attrs[0].Value.AsString() != "deploy-123" {
		t.Error("deployment ID attribute value incorrect")
	}
	if attrs[3].Value.AsInt64() != 42 {
		t.Error("user ID attribute value incorrect")
	}
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		Enabled:     true,
		Endpoint:    "collector:4317",
		ServiceName: "my-service",
		SampleRate:  0.5,
		Insecure:    false,
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.Endpoint != "collector:4317" {
		t.Error("Endpoint incorrect")
	}
	if cfg.ServiceName != "my-service" {
		t.Error("ServiceName incorrect")
	}
	if cfg.SampleRate != 0.5 {
		t.Error("SampleRate incorrect")
	}
	if cfg.Insecure {
		t.Error("Insecure should be false")
	}
}

func TestSpanKindOptions(t *testing.T) {
	// Verify SpanKind options are not nil
	if SpanKindServer == nil {
		t.Error("SpanKindServer is nil")
	}
	if SpanKindClient == nil {
		t.Error("SpanKindClient is nil")
	}
	if SpanKindInternal == nil {
		t.Error("SpanKindInternal is nil")
	}
}
