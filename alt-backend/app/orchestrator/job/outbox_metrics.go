package job

import (
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// These OTel counters are this file's fix for a specific gap: before this
// change nothing surfaced a stalled outbox — 5xx failures were marked FAILED
// with no metric, no alert and no health degradation, and the only way to
// notice was to query outbox_events directly. A sustained non-zero rate on
// the failed counter (reason "retries_exhausted" in particular) is the
// signal an alert should watch.
var (
	outboxMeterOnce            sync.Once
	outboxProcessedCounter     metric.Int64Counter
	outboxRetriedCounter       metric.Int64Counter
	outboxFailedCounter        metric.Int64Counter
	outboxReleaseFailedCounter metric.Int64Counter
)

func initOutboxMetrics() {
	outboxMeterOnce.Do(func() {
		meter := otel.Meter("alt-harvester.outbox-worker")
		outboxProcessedCounter, _ = meter.Int64Counter("alt_harvester_outbox_events_processed_total",
			metric.WithDescription("ARTICLE_UPSERT outbox events successfully delivered to RAG and knowledge-sovereign ArticleCreated"))
		outboxRetriedCounter, _ = meter.Int64Counter("alt_harvester_outbox_events_retried_total",
			metric.WithDescription("Outbox events released back to PENDING after a transient RAG upsert or ArticleCreated append failure"))
		outboxFailedCounter, _ = meter.Int64Counter("alt_harvester_outbox_events_failed_total",
			metric.WithDescription("Outbox events marked terminally FAILED, labeled by reason"))
		outboxReleaseFailedCounter, _ = meter.Int64Counter("alt_harvester_outbox_events_release_failed_total",
			metric.WithDescription("Outbox events where the release-to-PENDING RPC itself failed after a transient upsert failure, leaving the row stuck PROCESSING — invisible to both the PENDING claim query and a FAILED-status audit"))
	})
}
