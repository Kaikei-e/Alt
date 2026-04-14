package perf

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	meterOnce        sync.Once
	usecaseHist      otelmetric.Float64Histogram
	marshalHist      otelmetric.Float64Histogram
	totalHist        otelmetric.Float64Histogram
	rowCountHist     otelmetric.Float64Histogram
	cacheMissCount   otelmetric.Int64Counter
	dbHist           otelmetric.Float64Histogram
	mergeHist        otelmetric.Float64Histogram
	payloadBytesHist otelmetric.Float64Histogram
	tagCountHist     otelmetric.Float64Histogram
)

func initMetrics() {
	meterOnce.Do(func() {
		meter := otel.Meter("alt-backend.feed-read")
		usecaseHist, _ = meter.Float64Histogram("alt_feed_read_usecase_duration_ms",
			otelmetric.WithDescription("Usecase (DB+gateway) duration per endpoint"),
			otelmetric.WithUnit("ms"))
		marshalHist, _ = meter.Float64Histogram("alt_feed_read_marshal_duration_ms",
			otelmetric.WithDescription("Marshal duration per endpoint"),
			otelmetric.WithUnit("ms"))
		totalHist, _ = meter.Float64Histogram("alt_feed_read_total_duration_ms",
			otelmetric.WithDescription("Total handler duration per endpoint"),
			otelmetric.WithUnit("ms"))
		rowCountHist, _ = meter.Float64Histogram("alt_feed_read_row_count",
			otelmetric.WithDescription("Number of rows returned"))
		cacheMissCount, _ = meter.Int64Counter("alt_feed_read_cache_miss_total",
			otelmetric.WithDescription("Cache miss count (always increments until cache is implemented)"))
		dbHist, _ = meter.Float64Histogram("alt_feed_read_db_duration_ms",
			otelmetric.WithDescription("DB query duration per endpoint"),
			otelmetric.WithUnit("ms"))
		mergeHist, _ = meter.Float64Histogram("alt_feed_read_merge_duration_ms",
			otelmetric.WithDescription("Merge/enrichment duration per endpoint"),
			otelmetric.WithUnit("ms"))
		payloadBytesHist, _ = meter.Float64Histogram("alt_feed_read_payload_bytes",
			otelmetric.WithDescription("Estimated response payload size in bytes"),
			otelmetric.WithUnit("By"))
		tagCountHist, _ = meter.Float64Histogram("alt_feed_read_tag_count",
			otelmetric.WithDescription("Total tags across all rows"))
	})
}

// FeedReadTimings holds per-phase timing data for a single feed-read request.
type FeedReadTimings struct {
	Endpoint  string
	UsecaseMs int64
	MarshalMs int64
	TotalMs   int64
	RowCount  int
	// Phase 1 cache fields (always 0/false until cache is implemented)
	CacheMs  int64
	CacheHit bool
	// Phase 0 instrumentation fields
	DBMs         int64 // DB query time
	MergeMs      int64 // merge/enrichment time
	PayloadBytes int64 // estimated response payload size
	TagCount     int   // total tags across all rows
}

// FeedReadTimer measures per-phase durations and emits OTel spans + structured logs.
type FeedReadTimer struct {
	timings FeedReadTimings
	start   time.Time
	enabled bool
	logger  *slog.Logger
}

func isEnabled() bool {
	return os.Getenv("FEED_READ_PERF_ENABLED") != "false"
}

// NewFeedReadTimer creates a timer using the default slog logger.
func NewFeedReadTimer(endpoint string) *FeedReadTimer {
	return &FeedReadTimer{
		timings: FeedReadTimings{Endpoint: endpoint},
		start:   time.Now(),
		enabled: isEnabled(),
		logger:  slog.Default(),
	}
}

// NewFeedReadTimerWithLogger creates a timer with a custom logger (for testing).
func NewFeedReadTimerWithLogger(endpoint string, logger *slog.Logger) *FeedReadTimer {
	return &FeedReadTimer{
		timings: FeedReadTimings{Endpoint: endpoint},
		start:   time.Now(),
		enabled: isEnabled(),
		logger:  logger,
	}
}

// StartPhase begins timing a named phase and creates an OTel child span.
// Returns a stop function that must be called when the phase completes.
func (t *FeedReadTimer) StartPhase(ctx context.Context, name string) func() {
	if !t.enabled {
		return func() {}
	}

	spanName := "perf." + name
	_, span := otel.Tracer("alt-backend").Start(ctx, spanName)
	phaseStart := time.Now()

	return func() {
		elapsed := time.Since(phaseStart).Milliseconds()
		span.SetAttributes(attribute.Int64("duration_ms", elapsed))
		span.End()

		switch name {
		case "usecase":
			t.timings.UsecaseMs = elapsed
		case "marshal":
			t.timings.MarshalMs = elapsed
		case "cache":
			t.timings.CacheMs = elapsed
		case "db":
			t.timings.DBMs = elapsed
		case "merge":
			t.timings.MergeMs = elapsed
		}
	}
}

// SetRowCount records the number of rows returned.
func (t *FeedReadTimer) SetRowCount(n int) {
	t.timings.RowCount = n
}

// SetPayloadBytes records the estimated response payload size.
func (t *FeedReadTimer) SetPayloadBytes(n int64) {
	t.timings.PayloadBytes = n
}

// SetTagCount records the total tag count across all rows.
func (t *FeedReadTimer) SetTagCount(n int) {
	t.timings.TagCount = n
}

// Log emits a single structured log line with all timing fields and records OTel metrics.
func (t *FeedReadTimer) Log(ctx context.Context) {
	t.timings.TotalMs = time.Since(t.start).Milliseconds()

	if !t.enabled {
		return
	}

	// Record OTel metrics
	initMetrics()
	endpointAttr := otelmetric.WithAttributes(attribute.String("endpoint", t.timings.Endpoint))
	usecaseHist.Record(ctx, float64(t.timings.UsecaseMs), endpointAttr)
	marshalHist.Record(ctx, float64(t.timings.MarshalMs), endpointAttr)
	totalHist.Record(ctx, float64(t.timings.TotalMs), endpointAttr)
	rowCountHist.Record(ctx, float64(t.timings.RowCount), endpointAttr)
	dbHist.Record(ctx, float64(t.timings.DBMs), endpointAttr)
	mergeHist.Record(ctx, float64(t.timings.MergeMs), endpointAttr)
	payloadBytesHist.Record(ctx, float64(t.timings.PayloadBytes), endpointAttr)
	tagCountHist.Record(ctx, float64(t.timings.TagCount), endpointAttr)
	if !t.timings.CacheHit {
		cacheMissCount.Add(ctx, 1, endpointAttr)
	}

	// Emit structured log
	span := trace.SpanFromContext(ctx)
	logAttrs := []slog.Attr{
		slog.String("endpoint", t.timings.Endpoint),
		slog.Int64("usecase_ms", t.timings.UsecaseMs),
		slog.Int64("marshal_ms", t.timings.MarshalMs),
		slog.Int64("total_ms", t.timings.TotalMs),
		slog.Int("row_count", t.timings.RowCount),
		slog.Int64("cache_ms", t.timings.CacheMs),
		slog.Bool("cache_hit", t.timings.CacheHit),
		slog.Int64("db_ms", t.timings.DBMs),
		slog.Int64("merge_ms", t.timings.MergeMs),
		slog.Int64("payload_bytes", t.timings.PayloadBytes),
		slog.Int("tag_count", t.timings.TagCount),
	}

	if span.SpanContext().HasTraceID() {
		logAttrs = append(logAttrs, slog.String("trace_id", span.SpanContext().TraceID().String()))
	}

	args := make([]any, len(logAttrs))
	for i, a := range logAttrs {
		args[i] = a
	}

	t.logger.InfoContext(ctx, "feed_read_perf", args...)
}
