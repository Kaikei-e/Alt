package logger

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceContextHandler wraps an slog.Handler to automatically add trace context
// (trace_id and span_id) to log records when available.
//
// This enables logs output via stdout (JSON) to include trace correlation IDs,
// which are then collected by rask-log-forwarder and sent to ClickHouse.
type TraceContextHandler struct {
	inner slog.Handler
}

// NewTraceContextHandler creates a new TraceContextHandler wrapping the provided handler.
func NewTraceContextHandler(inner slog.Handler) *TraceContextHandler {
	return &TraceContextHandler{inner: inner}
}

// Enabled delegates to the inner handler.
func (h *TraceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle adds trace context to the record before delegating to the inner handler.
func (h *TraceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Extract trace context from context if available
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		sc := span.SpanContext()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a new handler with the given attributes added.
func (h *TraceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceContextHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup returns a new handler with the given group appended.
func (h *TraceContextHandler) WithGroup(name string) slog.Handler {
	return &TraceContextHandler{inner: h.inner.WithGroup(name)}
}
