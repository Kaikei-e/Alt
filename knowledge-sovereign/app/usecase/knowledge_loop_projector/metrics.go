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

	// relationCoverageTotal is the HONEST replacement for the
	// surface_planner_version=v2 ratio (ADR-000937). The v2 ratio tagged a
	// placement "v2" whenever a non-Null resolver was wired, even when every
	// placement was actually a v1 event-type fallback — so the dashboard could
	// read ~100% v2 while no real evidence reached the surface. This counter
	// instead labels each projected entry by whether it carries ≥1 first-class
	// relation. If producers stop emitting evidence, relation coverage drops to
	// ~0, surfacing the silent degradation the v2 ratio hid.
	relationCoverageTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "knowledge_loop",
			Subsystem: "projector",
			Name:      "relation_coverage_total",
			Help:      "Projected entries labelled by whether they carry at least one first-class relation (ADR-000937). The honest signal that evidence is reaching the Orient surface.",
		},
		[]string{"has_relations"},
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

	// ADR-000910 v2 observability surface (counters consumed by Phase H
	// dashboard panels + alert rules). All labels are bounded enums so the
	// Prometheus cardinality stays tractable.

	actOutcomeEmittedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "knowledge_loop",
			Subsystem: "projector",
			Name:      "act_outcome_emitted_total",
			Help:      "Number of KnowledgeLoopActOutcome events the projector observed, labelled by outcome enum (engaged / deep_engagement / stale_save / accepted_change / no_engagement).",
		},
		[]string{"outcome"},
	)

	lensModeSwitchedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "knowledge_loop",
			Subsystem: "projector",
			Name:      "lens_mode_switched_total",
			Help:      "Number of KnowledgeLoopLensModeSwitched events observed, labelled by from/to lens id.",
		},
		[]string{"from", "to"},
	)

	internalizedCountTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "knowledge_loop",
			Subsystem: "projector",
			Name:      "internalized_total",
			Help:      "Cumulative count of dismiss_state transitions to internalized (ADR-000908 §Δ3). 7-day rate is computed dashboard-side.",
		},
	)

	outcomeMissingTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "knowledge_loop",
			Subsystem: "projector",
			Name:      "outcome_missing_total",
			Help:      "Number of times act_outcome_cron filled an Acted event with outcome=no_engagement because no explicit outcome arrived inside the 7-day window. Rising rate signals immediate outcome emitter coverage gaps.",
		},
	)
)

// observeActOutcomeEmitted is called by the projector when a
// KnowledgeLoopActOutcome event is consumed.
func observeActOutcomeEmitted(outcome string) {
	actOutcomeEmittedTotal.WithLabelValues(outcome).Inc()
}

// observeLensModeSwitched is called when a KnowledgeLoopLensModeSwitched
// event is consumed. `from` may be "unspecified" on the first switch.
func observeLensModeSwitched(from, to string) {
	lensModeSwitchedTotal.WithLabelValues(from, to).Inc()
}

// observeInternalizedTransition is called by the projector when an entry's
// dismiss_state transitions to internalized.
func observeInternalizedTransition() {
	internalizedCountTotal.Inc()
}

// observeOutcomeMissingFill is called by act_outcome_cron when it backfills
// a no_engagement outcome after the 7-day window expires without an
// explicit outcome event.
func observeOutcomeMissingFill() {
	outcomeMissingTotal.Inc()
}

// ObserveOutcomeMissingFill is the exported entry point for the
// act_outcome_cron package (ADR-000908 §Δ1). The metric counter lives in
// the projector package so the /metrics endpoint exposes a single
// `knowledge_loop_projector_outcome_missing_total` series rather than two
// process-local registrations.
func ObserveOutcomeMissingFill() {
	observeOutcomeMissingFill()
}

// observeSurfaceBucketAssigned increments the bucket counter once per
// projected entry. Called by the projector's UPSERT-completion path in
// Wave 3 when decideBucketV2 starts producing v2 placements.
func observeSurfaceBucketAssigned(version string, bucket string) {
	surfaceBucketAssignedTotal.WithLabelValues(version, bucket).Inc()
}

// observeRelationCoverage records whether a projected entry carried any
// first-class relation. ADR-000937: this is the honest "is evidence reaching
// the Orient surface" signal, replacing the misleading v2-planner ratio.
func observeRelationCoverage(hasRelations bool) {
	label := "false"
	if hasRelations {
		label = "true"
	}
	relationCoverageTotal.WithLabelValues(label).Inc()
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
