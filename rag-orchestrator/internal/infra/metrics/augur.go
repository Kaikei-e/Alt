// Package metrics holds rag-orchestrator's Prometheus collectors.
//
// They are registered on the default registerer, which cmd/server exposes at
// /metrics on the API mux — the same surface the knowledge_event_emitter
// counters already use. PKI enrollment series live on a separate private
// registry and must not be mixed in here.
//
// Every series in this file exists because a postmortem asked for it: the Ask
// Augur answer path had no latency, no outcome breakdown and no corpus-health
// signal, so a degradation was only ever visible by reading a conversation.
package metrics

import (
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "rag_orchestrator"

// answerLatencyBuckets span the range a grounded RAG answer actually takes on
// this hardware: retrieval plus reranking alone runs into tens of seconds, so
// the default client_golang buckets (max 10s) would put almost every request
// in +Inf and make a p95 meaningless.
var answerLatencyBuckets = []float64{0.5, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144}

var (
	// streamTTFT measures request start → first content the user can read.
	// Heartbeats, progress and thinking frames are deliberately excluded:
	// they keep the connection alive without answering the question, and
	// counting them would report a healthy TTFT for a stream that has shown
	// the user nothing.
	streamTTFT = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "augur_stream",
		Name:      "ttft_seconds",
		Help: "Time from StreamChat request start to the first answer content " +
			"frame. Observed only when content is actually produced.",
		Buckets: answerLatencyBuckets,
	})

	// streamDuration measures the whole stream, labelled by how it ended, so
	// a fast failure is never mistaken for a fast answer.
	streamDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "augur_stream",
		Name:      "duration_seconds",
		Help:      "End-to-end StreamChat duration, by terminal outcome.",
		Buckets:   answerLatencyBuckets,
	}, []string{"outcome"})

	// answerTotal splits every finished answer two ways: how it was produced
	// (path) and how it ended (outcome). A degraded answer still succeeds from
	// the transport's point of view, which is why one label cannot carry both.
	answerTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "augur",
		Name:      "answer_total",
		Help: "Finished Ask Augur answers by production path " +
			"(normal, low_confidence, agentic_degraded, cache_hit) and terminal " +
			"outcome (success, fallback, error, clarification, cancelled).",
	}, []string{"path", "outcome"})

	// answerCacheTotal is the in-process answer cache's hit rate. A collapse
	// here explains a latency regression that looks like the LLM got slower.
	answerCacheTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "augur",
		Name:      "answer_cache_total",
		Help:      "Answer cache lookups by result (hit, miss).",
	}, []string{"result"})

	// rerankFallbackTotal counts answers served from contexts the
	// cross-encoder never ranked while reranking was configured. Those
	// contexts carry retrieval scores in a space the quality gates are not
	// calibrated for, which is the shape of the 2026-04-11 regression.
	rerankFallbackTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "augur",
		Name:      "rerank_fallback_total",
		Help: "Answers whose contexts were not ranked by the cross-encoder " +
			"although RERANK_ENABLED=true.",
	})

	// rerankExpectedTotal is rerankFallbackTotal's denominator: every answer
	// that should have been reranked. A rate needs both, and "answers" alone
	// includes the ones reranking was never configured for.
	rerankExpectedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "augur",
		Name:      "rerank_expected_total",
		Help:      "Answers for which cross-encoder reranking was configured.",
	})

	// emptyFallbackTotal counts degraded turns with no answer text. They are
	// deliberately not written to augur_messages (a blank assistant bubble is
	// worse than none), so this counter is the only trace they leave.
	emptyFallbackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "augur",
		Name:      "empty_fallback_total",
		Help: "Fallback turns that produced no answer text and were therefore " +
			"not persisted as an assistant message.",
	}, []string{"fallback_code"})
)

// Outcome values for streamDuration and answerTotal.
const (
	OutcomeSuccess       = "success"
	OutcomeFallback      = "fallback"
	OutcomeError         = "error"
	OutcomeClarification = "clarification"
	OutcomeCancelled     = "cancelled"
)

// Path values for answerTotal, in the priority the caller applies them: an
// answer that is both degraded and low-confidence is reported as degraded,
// because that is the condition an operator must act on.
const (
	PathNormal          = "normal"
	PathLowConfidence   = "low_confidence"
	PathAgenticDegraded = "agentic_degraded"
	PathCacheHit        = "cache_hit"
)

// rerankExpected mirrors RERANK_ENABLED. Without it a disabled reranker is
// indistinguishable from a broken one, and the fallback counter would run at
// 100% by design. Set once from the composition root.
var rerankExpected atomic.Bool

// ConfigureAugur records deployment facts the answer-path collectors need but
// cannot observe from a single request. Call from the composition root before
// serving; the accompanying startup log is what makes the setting auditable.
func ConfigureAugur(rerankEnabled bool, log *slog.Logger) {
	rerankExpected.Store(rerankEnabled)
	if log != nil {
		log.Info("augur_metrics_configured",
			slog.Bool("rerank_expected", rerankEnabled))
	}
}

// ObserveTTFT records time-to-first-content for one stream.
func ObserveTTFT(d time.Duration) { streamTTFT.Observe(d.Seconds()) }

// ObserveStreamDuration records a finished stream under its terminal outcome.
func ObserveStreamDuration(outcome string, d time.Duration) {
	streamDuration.WithLabelValues(outcome).Observe(d.Seconds())
}

// IncAnswer records one finished answer.
func IncAnswer(path, outcome string) {
	answerTotal.WithLabelValues(path, outcome).Inc()
}

// IncAnswerCache records one answer-cache lookup.
func IncAnswerCache(hit bool) {
	result := "miss"
	if hit {
		result = "hit"
	}
	answerCacheTotal.WithLabelValues(result).Inc()
}

// ObserveRerank records whether the cross-encoder ranked the contexts behind
// one answer. It is a no-op when reranking is not configured, so the fallback
// rate stays a measure of failure rather than of configuration.
func ObserveRerank(applied bool) {
	if !rerankExpected.Load() {
		return
	}
	rerankExpectedTotal.Inc()
	if !applied {
		rerankFallbackTotal.Inc()
	}
}

// IncEmptyFallback records a degraded turn that had no text to persist.
func IncEmptyFallback(fallbackCode string) {
	if fallbackCode == "" {
		fallbackCode = "unspecified"
	}
	emptyFallbackTotal.WithLabelValues(fallbackCode).Inc()
}
