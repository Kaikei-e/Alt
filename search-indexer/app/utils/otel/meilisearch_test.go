package otel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRecordMeilisearchProcessing_AttachesAttributesToActiveSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	original := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(original)

	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "parent")

	RecordMeilisearchProcessing(ctx, "SearchByUserID", 142)
	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}

	var sawProcessingMs, sawOp bool
	for _, attr := range spans[0].Attributes() {
		switch string(attr.Key) {
		case "meilisearch.processing_ms":
			sawProcessingMs = true
			if attr.Value.AsInt64() != 142 {
				t.Errorf("meilisearch.processing_ms = %d, want 142", attr.Value.AsInt64())
			}
		case "meilisearch.op":
			sawOp = true
			if attr.Value.AsString() != "SearchByUserID" {
				t.Errorf("meilisearch.op = %q, want SearchByUserID", attr.Value.AsString())
			}
		}
	}
	if !sawProcessingMs {
		t.Errorf("meilisearch.processing_ms attribute missing")
	}
	if !sawOp {
		t.Errorf("meilisearch.op attribute missing")
	}
}

// When there is no active span, RecordMeilisearchProcessing must not panic — the
// driver paths can be exercised from contexts without otel instrumentation
// (e.g. background indexers or tests).
func TestRecordMeilisearchProcessing_NoActiveSpan_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RecordMeilisearchProcessing panicked without active span: %v", r)
		}
	}()
	RecordMeilisearchProcessing(context.Background(), "Search", 1)
}
