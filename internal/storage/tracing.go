package storage

import (
	"context"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/metrics"
	"github.com/BlackOrder/vcdeploy/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracedOperation wraps a database operation with tracing and metrics.
func TracedOperation(ctx context.Context, operation, description string, fn func() error) error {
	_, span := tracing.Tracer("storage").Start(ctx, "db."+operation,
		trace.WithAttributes(
			attribute.String("db.operation", operation),
			attribute.String("db.statement", description),
		),
	)
	defer span.End()

	start := time.Now()
	err := fn()
	duration := time.Since(start).Seconds()

	// Record metrics
	metrics.DBQueryDuration.WithLabelValues(operation).Observe(duration)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// TracedQuery wraps a query operation with tracing and metrics.
func TracedQuery[T any](ctx context.Context, operation, description string, fn func() (T, error)) (T, error) {
	_, span := tracing.Tracer("storage").Start(ctx, "db."+operation,
		trace.WithAttributes(
			attribute.String("db.operation", operation),
			attribute.String("db.statement", description),
		),
	)
	defer span.End()

	start := time.Now()
	result, err := fn()
	duration := time.Since(start).Seconds()

	// Record metrics
	metrics.DBQueryDuration.WithLabelValues(operation).Observe(duration)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return result, err
}
