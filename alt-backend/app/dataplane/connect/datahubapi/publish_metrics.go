package datahubapi

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// CreateArticle publishes ArticleCreated / ArticleUpdated to mq-hub as a
// fire-and-forget side effect: a failure is logged WARN and the RPC still
// answers success, because the durable article write already landed and a
// hard-fail here would reject a good ingest. That trade-off is deliberate
// (a proper outbox for these publishes is the larger fix, tracked separately),
// but a silent WARN alone means a sustained mq-hub outage drains downstream
// summarisation/indexing with nothing in the metrics to alert on. This counter
// makes the loss observable: a non-zero rate is the signal.
var (
	publishMetricsOnce          sync.Once
	articlePublishFailedCounter metric.Int64Counter
)

func initPublishMetrics() {
	publishMetricsOnce.Do(func() {
		meter := otel.Meter("alt-data-hub.datahubapi")
		articlePublishFailedCounter, _ = meter.Int64Counter(
			"alt_datahub_article_publish_failed_total",
			metric.WithDescription("CreateArticle mq-hub publishes that failed (fire-and-forget, RPC still succeeded), labeled by event type"),
		)
	})
}

// recordArticlePublishFailure increments the fire-and-forget publish-failure
// counter. eventType is "article_created" or "article_updated".
func recordArticlePublishFailure(ctx context.Context, eventType string) {
	initPublishMetrics()
	if articlePublishFailedCounter == nil {
		return
	}
	articlePublishFailedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("event_type", eventType)))
}
