package knowledge_loop

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Streaming counters for the Knowledge Loop SSE handler.
//
// Pair with:
//
//	observability/prometheus/rules/knowledge-loop-rules.yml (alerts)
//	observability/grafana/dashboards/knowledge-loop-projector.json (panels)
//
// Labels are kept low-cardinality on purpose: only termination reason is
// keyed on streamEndedTotal so dashboards can decompose by cause without
// blowing up series count. No user / tenant labels — those belong to logs.
var (
	streamStartedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "alt_knowledge_loop",
			Subsystem: "stream",
			Name:      "started_total",
			Help:      "Number of StreamKnowledgeLoopUpdates server-side sessions opened.",
		},
	)

	streamEndedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "alt_knowledge_loop",
			Subsystem: "stream",
			Name:      "ended_total",
			Help:      "Number of StreamKnowledgeLoopUpdates sessions terminated, labelled by reason (ctx_done | jwt_expired | stale | upstream_error).",
		},
		[]string{"reason"},
	)

	streamFetchFailedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "alt_knowledge_loop",
			Subsystem: "stream",
			Name:      "fetch_failed_total",
			Help:      "Number of upstream fetch failures inside an active stream session.",
		},
	)
)
