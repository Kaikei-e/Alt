package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RecordMeilisearchProcessing attaches Meilisearch's self-reported
// processingTimeMs to the active span as a sideband signal so we can
// separate Meilisearch processing time (which includes embedder calls for
// hybrid search) from the end-to-end Connect-RPC handler latency without
// threading the value through gateway / port / usecase layers.
//
// When no span is active (e.g. background indexers) the call is a no-op.
func RecordMeilisearchProcessing(ctx context.Context, op string, processingMs int64) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("meilisearch.op", op),
		attribute.Int64("meilisearch.processing_ms", processingMs),
	)
}
