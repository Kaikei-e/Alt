package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTraceContextHandler_Handle_WithValidSpan(t *testing.T) {
	// Setup in-memory span exporter
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(provider)
	defer func() { _ = provider.Shutdown(context.Background()) }()

	// Create a buffer to capture JSON output
	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := NewTraceContextHandler(jsonHandler)
	logger := slog.New(handler)

	// Create a span and log within its context
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	logger.InfoContext(ctx, "test message")

	// Parse the JSON output
	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Verify trace_id and span_id are present
	traceID, ok := logEntry["trace_id"].(string)
	if !ok || traceID == "" {
		t.Error("Expected trace_id to be present and non-empty")
	}
	if traceID == "00000000000000000000000000000000" {
		t.Error("Expected trace_id to be a valid ID, not all zeros")
	}

	spanID, ok := logEntry["span_id"].(string)
	if !ok || spanID == "" {
		t.Error("Expected span_id to be present and non-empty")
	}
	if spanID == "0000000000000000" {
		t.Error("Expected span_id to be a valid ID, not all zeros")
	}

	// Verify the message is present
	msg, ok := logEntry["msg"].(string)
	if !ok || msg != "test message" {
		t.Errorf("Expected msg to be 'test message', got '%v'", msg)
	}
}

func TestTraceContextHandler_Handle_WithoutSpan(t *testing.T) {
	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := NewTraceContextHandler(jsonHandler)
	logger := slog.New(handler)

	// Log without a span in context
	logger.Info("test message without span")

	// Parse the JSON output
	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Verify trace_id and span_id are NOT present
	if _, ok := logEntry["trace_id"]; ok {
		t.Error("Expected trace_id to be absent when no span in context")
	}
	if _, ok := logEntry["span_id"]; ok {
		t.Error("Expected span_id to be absent when no span in context")
	}

	// Verify the message is present
	msg, ok := logEntry["msg"].(string)
	if !ok || msg != "test message without span" {
		t.Errorf("Expected msg to be 'test message without span', got '%v'", msg)
	}
}

func TestTraceContextHandler_Enabled(t *testing.T) {
	jsonHandler := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := NewTraceContextHandler(jsonHandler)

	// Should be enabled for INFO and above
	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected INFO level to be enabled")
	}
	if !handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Expected WARN level to be enabled")
	}
	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Error("Expected ERROR level to be enabled")
	}
	// Should NOT be enabled for DEBUG when handler is set to INFO
	if handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected DEBUG level to be disabled")
	}
}

func TestTraceContextHandler_WithAttrs(t *testing.T) {
	jsonHandler := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := NewTraceContextHandler(jsonHandler)

	// Create new handler with attrs
	newHandler := handler.WithAttrs([]slog.Attr{slog.String("key", "value")})

	// Verify it returns a new handler
	if newHandler == handler {
		t.Error("WithAttrs should return a new handler instance")
	}

	// Verify it's still a TraceContextHandler
	if _, ok := newHandler.(*TraceContextHandler); !ok {
		t.Error("WithAttrs should return a TraceContextHandler")
	}
}

func TestTraceContextHandler_WithGroup(t *testing.T) {
	jsonHandler := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := NewTraceContextHandler(jsonHandler)

	// Create new handler with group
	newHandler := handler.WithGroup("testgroup")

	// Verify it returns a new handler
	if newHandler == handler {
		t.Error("WithGroup should return a new handler instance")
	}

	// Verify it's still a TraceContextHandler
	if _, ok := newHandler.(*TraceContextHandler); !ok {
		t.Error("WithGroup should return a TraceContextHandler")
	}
}

func TestTraceContextHandler_WithAttrs_PreservesTraceContext(t *testing.T) {
	// Setup in-memory span exporter
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(provider)
	defer func() { _ = provider.Shutdown(context.Background()) }()

	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := NewTraceContextHandler(jsonHandler)
	logger := slog.New(handler).With("service", "test-service")

	// Create a span and log within its context
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	logger.InfoContext(ctx, "test message with attrs")

	// Parse the JSON output
	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Verify both the attr and trace context are present
	if service, ok := logEntry["service"].(string); !ok || service != "test-service" {
		t.Errorf("Expected service attr to be 'test-service', got '%v'", logEntry["service"])
	}
	if _, ok := logEntry["trace_id"].(string); !ok {
		t.Error("Expected trace_id to be present when using WithAttrs")
	}
	if _, ok := logEntry["span_id"].(string); !ok {
		t.Error("Expected span_id to be present when using WithAttrs")
	}
}
