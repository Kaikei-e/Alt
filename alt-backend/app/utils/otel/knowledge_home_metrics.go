package otel

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const khMeterName = "alt-backend"

// KnowledgeHomeMetrics holds all OTel metrics for Knowledge Home.
type KnowledgeHomeMetrics struct {
	// Snapshot provides atomic read access for the admin API.
	Snapshot *MetricsSnapshot

	// Projector metrics
	ProjectorEventsProcessed metric.Int64Counter
	ProjectorLagSeconds      metric.Float64Gauge
	ProjectorBatchDurationMs metric.Float64Histogram
	ProjectorErrors          metric.Int64Counter

	// Handler metrics
	PageServed   metric.Int64Counter
	PageDegraded metric.Int64Counter

	// Tracking metrics
	ItemsExposed   metric.Int64Counter
	ItemsOpened    metric.Int64Counter
	ItemsDismissed metric.Int64Counter

	// Backfill metrics
	BackfillEventsGenerated metric.Int64Counter

	// SLI-A: availability
	RequestsTotal          metric.Int64Counter
	RequestDurationSeconds metric.Float64Histogram
	DegradedResponsesTotal metric.Int64Counter
	ProjectionAgeSeconds   metric.Float64Gauge

	// SLI-C: durability
	TrackingReceivedTotal  metric.Int64Counter
	TrackingPersistedTotal metric.Int64Counter
	TrackingFailedTotal    metric.Int64Counter

	// SLI-D: stream
	StreamConnectionsTotal metric.Int64Counter
	StreamDisconnectsTotal metric.Int64Counter
	StreamReconnectsTotal  metric.Int64Counter
	StreamDeliveriesTotal  metric.Int64Counter
	StreamUpdateLagSeconds metric.Float64Histogram

	// SLI-E: correctness
	EmptyResponsesTotal    metric.Int64Counter
	MalformedWhyTotal      metric.Int64Counter
	OrphanItemsTotal       metric.Int64Counter
	SupersedeMismatchTotal metric.Int64Counter

	// Reproject
	ReprojectEventsTotal metric.Int64Counter

	// Sovereign (remote service)
	SovereignMutationApplied  metric.Int64Counter
	SovereignMutationError    metric.Int64Counter
	SovereignMutationDuration metric.Float64Histogram

	// Recall
	RecallSignalAppendTotal       metric.Int64Counter
	RecallSignalAppendErrors      metric.Int64Counter
	RecallCandidateGeneratedTotal metric.Int64Counter
	RecallCandidateEmptyTotal     metric.Int64Counter
	RecallProjectorDurationMs     metric.Float64Histogram
	RecallProjectorUsersProcessed metric.Int64Counter
}

// NewKnowledgeHomeMetrics initializes all Knowledge Home OTel metrics.
func NewKnowledgeHomeMetrics() (*KnowledgeHomeMetrics, error) {
	meter := otel.Meter(khMeterName)
	m := &KnowledgeHomeMetrics{
		Snapshot: NewMetricsSnapshot(),
	}
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

	// SLI-A: availability
	m.RequestsTotal, err = meter.Int64Counter("alt_home_requests_total",
		metric.WithDescription("SLI-A: availability"))
	if err != nil {
		return nil, err
	}
	m.RequestDurationSeconds, err = meter.Float64Histogram("alt_home_request_duration_seconds",
		metric.WithDescription("latency budget"))
	if err != nil {
		return nil, err
	}
	m.DegradedResponsesTotal, err = meter.Int64Counter("alt_home_degraded_responses_total",
		metric.WithDescription("degraded tracking"))
	if err != nil {
		return nil, err
	}
	m.ProjectionAgeSeconds, err = meter.Float64Gauge("alt_home_projection_age_seconds",
		metric.WithDescription("freshness"))
	if err != nil {
		return nil, err
	}

	// SLI-C: durability
	m.TrackingReceivedTotal, err = meter.Int64Counter("alt_home_tracking_received_total",
		metric.WithDescription("SLI-C: durability"))
	if err != nil {
		return nil, err
	}
	m.TrackingPersistedTotal, err = meter.Int64Counter("alt_home_tracking_persisted_total",
		metric.WithDescription("durability"))
	if err != nil {
		return nil, err
	}
	m.TrackingFailedTotal, err = meter.Int64Counter("alt_home_tracking_failed_total",
		metric.WithDescription("durability"))
	if err != nil {
		return nil, err
	}

	// SLI-D: stream
	m.StreamConnectionsTotal, err = meter.Int64Counter("alt_home_stream_connections_total",
		metric.WithDescription("SLI-D: stream"))
	if err != nil {
		return nil, err
	}
	m.StreamDisconnectsTotal, err = meter.Int64Counter("alt_home_stream_disconnects_total",
		metric.WithDescription("stream"))
	if err != nil {
		return nil, err
	}
	m.StreamReconnectsTotal, err = meter.Int64Counter("alt_home_stream_reconnects_total",
		metric.WithDescription("stream"))
	if err != nil {
		return nil, err
	}
	m.StreamDeliveriesTotal, err = meter.Int64Counter("alt_home_stream_deliveries_total",
		metric.WithDescription("successful non-heartbeat stream deliveries"))
	if err != nil {
		return nil, err
	}
	m.StreamUpdateLagSeconds, err = meter.Float64Histogram("alt_home_stream_update_lag_seconds",
		metric.WithDescription("stream"))
	if err != nil {
		return nil, err
	}

	// SLI-E: correctness
	m.EmptyResponsesTotal, err = meter.Int64Counter("alt_home_empty_responses_total",
		metric.WithDescription("SLI-E: correctness"))
	if err != nil {
		return nil, err
	}
	m.MalformedWhyTotal, err = meter.Int64Counter("alt_home_malformed_why_total",
		metric.WithDescription("correctness"))
	if err != nil {
		return nil, err
	}
	m.OrphanItemsTotal, err = meter.Int64Counter("alt_home_orphan_items_total",
		metric.WithDescription("correctness"))
	if err != nil {
		return nil, err
	}
	m.SupersedeMismatchTotal, err = meter.Int64Counter("alt_home_supersede_mismatch_total",
		metric.WithDescription("correctness"))
	if err != nil {
		return nil, err
	}

	// Reproject
	m.ReprojectEventsTotal, err = meter.Int64Counter("alt_home_reproject_events_total",
		metric.WithDescription("reproject"))
	if err != nil {
		return nil, err
	}

	// Sovereign
	m.SovereignMutationApplied, err = meter.Int64Counter("alt.sovereign.mutation.applied_total",
		metric.WithDescription("Sovereign mutation applications by type"))
	if err != nil {
		return nil, err
	}
	m.SovereignMutationError, err = meter.Int64Counter("alt.sovereign.mutation.error_total",
		metric.WithDescription("Sovereign mutation errors by type"))
	if err != nil {
		return nil, err
	}
	m.SovereignMutationDuration, err = meter.Float64Histogram("alt.sovereign.mutation.duration_ms",
		metric.WithDescription("Sovereign mutation duration in ms"))
	if err != nil {
		return nil, err
	}

	// Recall
	m.RecallSignalAppendTotal, err = meter.Int64Counter("alt.home.recall.signal_append_total",
		metric.WithDescription("Recall signals appended by type"))
	if err != nil {
		return nil, err
	}
	m.RecallSignalAppendErrors, err = meter.Int64Counter("alt.home.recall.signal_append_errors",
		metric.WithDescription("Recall signal append failures"))
	if err != nil {
		return nil, err
	}
	m.RecallCandidateGeneratedTotal, err = meter.Int64Counter("alt.home.recall.candidate_generated_total",
		metric.WithDescription("Recall candidates generated by projector"))
	if err != nil {
		return nil, err
	}
	m.RecallCandidateEmptyTotal, err = meter.Int64Counter("alt.home.recall.candidate_empty_total",
		metric.WithDescription("Users with zero recall candidates after projection"))
	if err != nil {
		return nil, err
	}
	m.RecallProjectorDurationMs, err = meter.Float64Histogram("alt.home.recall.projector_duration_ms",
		metric.WithDescription("Duration of recall projector run in ms"))
	if err != nil {
		return nil, err
	}
	m.RecallProjectorUsersProcessed, err = meter.Int64Counter("alt.home.recall.projector_users_processed",
		metric.WithDescription("Users processed by recall projector"))
	if err != nil {
		return nil, err
	}

	return m, nil
}
