package otel

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const khMeterName = "alt-backend"

// KnowledgeHomeMetrics holds all OTel metrics for Knowledge Home.
type KnowledgeHomeMetrics struct {
	// Projector metrics
	ProjectorEventsProcessed metric.Int64Counter
	ProjectorLagSeconds      metric.Float64Gauge
	ProjectorBatchDurationMs metric.Float64Histogram
	ProjectorErrors          metric.Int64Counter

	// Handler metrics
	PageServed  metric.Int64Counter
	PageDegraded metric.Int64Counter

	// Tracking metrics
	ItemsExposed  metric.Int64Counter
	ItemsOpened   metric.Int64Counter
	ItemsDismissed metric.Int64Counter

	// Backfill metrics
	BackfillEventsGenerated metric.Int64Counter
}

// NewKnowledgeHomeMetrics initializes all Knowledge Home OTel metrics.
func NewKnowledgeHomeMetrics() (*KnowledgeHomeMetrics, error) {
	meter := otel.Meter(khMeterName)
	m := &KnowledgeHomeMetrics{}
	var err error

	// Projector
	m.ProjectorEventsProcessed, err = meter.Int64Counter("alt.home.projector.events_processed",
		metric.WithDescription("Number of knowledge events processed by projector"))
	if err != nil {
		return nil, err
	}
	m.ProjectorLagSeconds, err = meter.Float64Gauge("alt.home.projector.lag_seconds",
		metric.WithDescription("Seconds since last processed event"))
	if err != nil {
		return nil, err
	}
	m.ProjectorBatchDurationMs, err = meter.Float64Histogram("alt.home.projector.batch_duration_ms",
		metric.WithDescription("Duration of projector batch processing in ms"))
	if err != nil {
		return nil, err
	}
	m.ProjectorErrors, err = meter.Int64Counter("alt.home.projector.errors",
		metric.WithDescription("Number of projector errors"))
	if err != nil {
		return nil, err
	}

	// Handler
	m.PageServed, err = meter.Int64Counter("alt.home.page.served",
		metric.WithDescription("Number of Knowledge Home pages served"))
	if err != nil {
		return nil, err
	}
	m.PageDegraded, err = meter.Int64Counter("alt.home.page.degraded",
		metric.WithDescription("Number of degraded Knowledge Home pages"))
	if err != nil {
		return nil, err
	}

	// Tracking
	m.ItemsExposed, err = meter.Int64Counter("alt.home.items.exposed",
		metric.WithDescription("Number of items exposed (seen) on Knowledge Home"))
	if err != nil {
		return nil, err
	}
	m.ItemsOpened, err = meter.Int64Counter("alt.home.items.opened",
		metric.WithDescription("Number of items opened from Knowledge Home"))
	if err != nil {
		return nil, err
	}
	m.ItemsDismissed, err = meter.Int64Counter("alt.home.items.dismissed",
		metric.WithDescription("Number of items dismissed from Knowledge Home"))
	if err != nil {
		return nil, err
	}

	// Backfill
	m.BackfillEventsGenerated, err = meter.Int64Counter("alt.home.backfill.events_generated",
		metric.WithDescription("Number of backfill events generated"))
	if err != nil {
		return nil, err
	}

	return m, nil
}
