package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Distinct from knowledge_loop_projector_event_dropped_total so emitter degradation is not masked by or attributed to projector behavior.
var knowledgeEventEmitterFailureTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "knowledge_event_emitter",
		Name:      "failure_total",
		Help: "Number of knowledge_events emit attempts that failed at the " +
			"knowledge event emitter seam (RPC error, timeout, or input rejection). " +
			"Distinct from projector-side drops; warn-and-continue keeps the " +
			"caller flow alive but bumps this counter so the rollout is observable.",
	},
	[]string{"event_type"},
)

// IncEmitterFailure records one emit failure for the given canonical
// event_type. Callers should pass the wire-format event_type string so
// dashboards can split by signal kind (augur link vs recap topic).
func IncEmitterFailure(eventType string) {
	knowledgeEventEmitterFailureTotal.WithLabelValues(eventType).Inc()
}
