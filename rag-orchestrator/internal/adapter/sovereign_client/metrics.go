package sovereign_client

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// knowledgeEventEmitterFailureTotal counts knowledge_events emit attempts
// that failed at the sovereign_client adapter seam — RPC errors, timeouts,
// validation rejections, anything that prevents the event from reaching
// knowledge-sovereign.
//
// This is deliberately a different counter from the projector's
// knowledge_loop_projector_event_dropped_total: that one tracks
// drops *after* an event is appended (e.g. user_mismatch in the
// projector). Splitting them means an emitter degradation in one
// service doesn't get masked by, or attributed to, projector behaviour.
//
// Cardinality: event_type is bounded by the canonical contract §6.4.1
// allowlist (currently augur.conversation_linked.v1; recap.topic_snapshotted.v1
// will follow once recap-worker integrates the persist-stage emit). Add
// new event types only when they are emitted from rag-orchestrator.
var knowledgeEventEmitterFailureTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "rag_orchestrator",
		Subsystem: "knowledge_event_emitter",
		Name:      "failure_total",
		Help: "Number of knowledge_events emit attempts that failed at the " +
			"sovereign_client adapter (RPC error, timeout, or input rejection). " +
			"Distinct from projector-side drops; warn-and-continue keeps the " +
			"caller flow alive but bumps this counter so the rollout is observable.",
	},
	[]string{"event_type"},
)

// IncEmitterFailure records one emit failure for the given canonical
// event_type. Callers should pass the wire-format event_type string so
// dashboards can split by signal kind (augur link vs recap topic).
//
// The function intentionally stays small and side-effect-free beyond the
// counter increment — handlers stay in charge of warn logging because
// they hold the contextual fields (entry_key, conversation_id) the metric
// must not carry as labels.
func IncEmitterFailure(eventType string) {
	knowledgeEventEmitterFailureTotal.WithLabelValues(eventType).Inc()
}
