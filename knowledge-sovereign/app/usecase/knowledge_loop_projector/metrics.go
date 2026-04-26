package knowledge_loop_projector

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// Metric vocabulary for the Knowledge Loop projector. All counters are
// process-local prometheus collectors registered with the default registry
// so the /metrics endpoint exposes them automatically.
//
// Metrics are intentionally additive — every label is a low-cardinality
// enum (event_type stays bounded by the constants in this package, bucket
// is the proto enum, version is "v1" | "v2"). Wave 3+ may add histograms,
// but the counters below are the minimum viable observability surface for
// Surface Planner v2 and change_summary redline rollout.
//
// Why each counter exists:
//   - surfaceBucketAssignedTotal: confirms the projector is actually
//     placing entries via decideBucketV2 once Wave 3 wires the resolver.
//     The {version,bucket} label split lets dashboards spot the v1 → v2
//     migration drift in real time.
//   - changeSummaryWrittenTotal: traces ChangedDiffCard's rollout from
//     legacy Then/Now to redline-proof. The redline_capable label tracks
//     when the SummarySuperseded emitter starts including excerpts.
//   - eventDroppedTotal: F-002 / F-003 enforcement. Any time the projector
//     drops an event for safety reasons (e.g. user/article mismatch), we
//     bump this counter so dashboards / alerts pick it up. Normally 0.
//   - crossUserIsolationViolationTotal: F-001 enforcement. This counter
//     should always be 0; an alert on > 0 catches a leak immediately.
var (
	surfaceBucketAssignedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "knowledge_loop",
			Subsystem: "projector",
			Name:      "surface_bucket_assigned_total",
			Help:      "Number of Knowledge Loop entries assigned to a surface bucket by the projector, labelled by planner version and bucket.",
		},
		[]string{"version", "bucket"},
	)

	changeSummaryWrittenTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "knowledge_loop",
			Subsystem: "projector",
			Name:      "change_summary_written_total",
			Help:      "Number of change_summary JSONB blobs written by the SummarySuperseded path, labelled by whether the payload had redline-capable excerpts.",
		},
		[]string{"redline_capable"},
	)

	eventDroppedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "knowledge_loop",
			Subsystem: "projector",
			Name:      "event_dropped_total",
			Help:      "Number of events the projector dropped without persisting, labelled by reason.",
		},
		[]string{"reason"},
	)

	crossUserIsolationViolationTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "knowledge_loop",
			Subsystem: "projector",
			Name:      "cross_user_isolation_violation_total",
			Help:      "F-001 guard: number of times the projector detected a cross-user evidence reference and rejected it. Should always be 0.",
		},
	)
)

// observeSurfaceBucketAssigned increments the bucket counter once per
// projected entry. Called by the projector's UPSERT-completion path in
// Wave 3 when decideBucketV2 starts producing v2 placements.
func observeSurfaceBucketAssigned(version string, bucket string) {
	surfaceBucketAssignedTotal.WithLabelValues(version, bucket).Inc()
}

// observeChangeSummaryWritten increments the change_summary counter every
// time the projector writes a non-empty change_summary blob. The
// redlineCapable bool tracks whether the payload carried excerpts that
// drove computeChangeDiff.
func observeChangeSummaryWritten(redlineCapable bool) {
	label := "false"
	if redlineCapable {
		label = "true"
	}
	changeSummaryWrittenTotal.WithLabelValues(label).Inc()
}

// observeEventDropped increments the drop counter with a stable reason
// label. Allowed reason values must stay low-cardinality so dashboards
// don't blow up — keep them in the constant block below.
func observeEventDropped(reason string) {
	eventDroppedTotal.WithLabelValues(reason).Inc()
}

// Drop reasons. Adding a new reason requires updating the dashboard panel
// that filters on these values.
const (
	DropReasonNoUserID         = "no_user_id"
	DropReasonInvalidEntryKey  = "invalid_entry_key"
	DropReasonUserMismatch     = "user_mismatch"
	DropReasonArticleMismatch  = "article_mismatch"
	DropReasonUnknownEventType = "unknown_event_type"
)

// bucketMetricLabel maps the SurfaceBucket proto enum onto the lower-case
// label values dashboards expect. The label vocabulary stays bounded to
// the four canonical buckets — anything else would inflate cardinality
// and signal an upstream bug.
func bucketMetricLabel(b sovereignv1.SurfaceBucket) string {
	switch b {
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW:
		return "now"
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE:
		return "continue"
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED:
		return "changed"
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW:
		return "review"
	default:
		return "unspecified"
	}
}
