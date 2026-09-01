package augur

import (
	"time"

	"rag-orchestrator/internal/infra/metrics"
	"rag-orchestrator/internal/usecase"
)

// streamTelemetry accumulates what one StreamChat call is worth measuring, and
// emits it once at the end.
//
// It reads the stream rather than the usecase internals on purpose: the metric
// that matters is the one describing what the caller experienced, and the
// handler is the only place that sees both the frames on the wire and the
// terminal answer behind them.
type streamTelemetry struct {
	start        time.Time
	firstContent time.Time
	outcome      string
	path         string
	// rerankKnown guards the rerank observation: only the Done event carries
	// the flag, and a stream that never reaches Done says nothing about the
	// cross-encoder either way.
	rerankKnown   bool
	rerankApplied bool
	cacheKnown    bool
	cacheHit      bool
}

func newStreamTelemetry() *streamTelemetry {
	return &streamTelemetry{
		start:   time.Now(),
		outcome: metrics.OutcomeError,
		path:    metrics.PathNormal,
	}
}

// markFirstContent records time-to-first-token once. Later content frames do
// not move it; heartbeats, progress and thinking frames never reach it.
func (t *streamTelemetry) markFirstContent() {
	if t.firstContent.IsZero() {
		t.firstContent = time.Now()
	}
}

// observeEvent classifies the stream from the frames it produces. The terminal
// frame wins: a stream that streamed deltas and then declined is a fallback.
func (t *streamTelemetry) observeEvent(event usecase.StreamEvent) {
	switch event.Kind {
	case usecase.StreamEventKindFallback:
		t.outcome = metrics.OutcomeFallback
	case usecase.StreamEventKindError:
		t.outcome = metrics.OutcomeError
	case usecase.StreamEventKindClarification:
		t.outcome = metrics.OutcomeClarification
	case usecase.StreamEventKindDone:
		output, ok := event.Payload.(*usecase.AnswerWithRAGOutput)
		if !ok || output == nil {
			return
		}
		if output.Answer != "" {
			t.markFirstContent()
		}
		switch {
		case output.Fallback:
			t.outcome = metrics.OutcomeFallback
		case t.outcome == metrics.OutcomeClarification:
			// Clarification already decided; the Done event carries no answer.
		default:
			t.outcome = metrics.OutcomeSuccess
		}
		t.path = answerPath(output.Debug)
		t.cacheKnown, t.cacheHit = true, output.Debug.CacheHit
		// A cached answer replays contexts that were reranked (or not) on the
		// original request; counting it again would double-report that run.
		if !output.Debug.CacheHit {
			t.rerankKnown, t.rerankApplied = true, output.Debug.RerankApplied
		}
	}
}

func (t *streamTelemetry) finish() {
	if !t.firstContent.IsZero() {
		metrics.ObserveTTFT(t.firstContent.Sub(t.start))
	}
	metrics.ObserveStreamDuration(t.outcome, time.Since(t.start))
	metrics.IncAnswer(t.path, t.outcome)
	if t.cacheKnown {
		metrics.IncAnswerCache(t.cacheHit)
	}
	if t.rerankKnown {
		metrics.ObserveRerank(t.rerankApplied)
	}
}

// answerPath names how the answer was produced. The order is a priority, not a
// preference: a degraded agentic run is the condition an operator has to act
// on, so it outranks the softer low-confidence and cache labels.
func answerPath(debug usecase.AnswerDebug) string {
	switch {
	case debug.AgenticDegraded:
		return metrics.PathAgenticDegraded
	case debug.LowConfidence:
		return metrics.PathLowConfidence
	case debug.CacheHit:
		return metrics.PathCacheHit
	default:
		return metrics.PathNormal
	}
}
