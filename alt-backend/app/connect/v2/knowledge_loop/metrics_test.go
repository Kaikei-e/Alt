package knowledge_loop

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// PM-2026-045 actions #3 / #4 require streaming churn and JWT expiry to be
// observable as Prometheus counters, not just structured logs. The handler
// already emits slog records for stream_started / stream_jwt_expired /
// stream_ended / stream_fetch_failed; these counters give the alert layer
// (observability/prometheus/rules/knowledge-loop-rules.yml) a direct signal
// that survives log aggregation lag.
//
// The labels are intentionally low-cardinality:
//   - reason on streamEndedTotal mirrors the structured-log reason values
//     so dashboards can decompose by termination cause.
//   - no per-user labels — that would blow up cardinality.

func TestStreamStartedTotal_IncrementsWithoutLabels(t *testing.T) {
	t.Parallel()

	before := testutil.ToFloat64(streamStartedTotal)
	streamStartedTotal.Inc()
	if got := testutil.ToFloat64(streamStartedTotal) - before; got != 1 {
		t.Fatalf("stream_started_total: expected delta 1, got %v", got)
	}
}

func TestStreamEndedTotal_IncrementsByReason(t *testing.T) {
	t.Parallel()

	reasons := []string{"ctx_done", "jwt_expired", "stale", "upstream_error"}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			counter := streamEndedTotal.WithLabelValues(reason)
			before := testutil.ToFloat64(counter)
			counter.Inc()
			if got := testutil.ToFloat64(counter) - before; got != 1 {
				t.Fatalf("stream_ended_total{reason=%q}: expected delta 1, got %v", reason, got)
			}
		})
	}
}

func TestStreamFetchFailedTotal_IncrementsWithoutLabels(t *testing.T) {
	t.Parallel()

	before := testutil.ToFloat64(streamFetchFailedTotal)
	streamFetchFailedTotal.Inc()
	if got := testutil.ToFloat64(streamFetchFailedTotal) - before; got != 1 {
		t.Fatalf("stream_fetch_failed_total: expected delta 1, got %v", got)
	}
}
