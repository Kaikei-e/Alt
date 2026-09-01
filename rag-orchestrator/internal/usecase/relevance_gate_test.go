package usecase

import (
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func makeCtx(title string, rerankScore float32) ContextItem {
	return ContextItem{
		ChunkText:     "chunk text for " + title,
		Title:         title,
		Score:         rerankScore,
		ScoreKind:     domain.ScoreKindRerank,
		RerankScore:   rerankScore,
		RerankApplied: true,
		ChunkID:       uuid.New(),
	}
}

func TestRelevanceGate_Good(t *testing.T) {
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		makeCtx("Example Systems escalates recalls", 0.82),
		makeCtx("Oil supply concerns", 0.65),
	}
	assert.Equal(t, QualityGood, gate.Evaluate(contexts))
}

func TestRelevanceGate_Marginal(t *testing.T) {
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		makeCtx("Vaguely related article", 0.35),
		makeCtx("Another article", 0.20),
	}
	assert.Equal(t, QualityMarginal, gate.Evaluate(contexts))
}

func TestRelevanceGate_Insufficient(t *testing.T) {
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		makeCtx("Orchard Gardening Weekly", 0.10),
		makeCtx("Example Protocol Digest", 0.05),
	}
	assert.Equal(t, QualityInsufficient, gate.Evaluate(contexts))
}

func TestRelevanceGate_EmptyContexts(t *testing.T) {
	gate := NewRelevanceGate(0.5, 0.25)
	assert.Equal(t, QualityInsufficient, gate.Evaluate(nil))
	assert.Equal(t, QualityInsufficient, gate.Evaluate([]ContextItem{}))
}

func TestRelevanceGate_WithoutRerank_ReportsMarginalWhateverTheScore(t *testing.T) {
	// Score without RerankApplied is a retrieval score on an unrelated scale
	// (RRF, or a raw vector distance), so its magnitude carries no verdict —
	// neither a high one nor a low one. See the RRF regression below.
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		{ChunkText: "text", Title: "title", Score: 0.7, RerankScore: 0, ChunkID: uuid.New()},
	}
	assert.Equal(t, QualityMarginal, gate.Evaluate(contexts))
}

func TestRelevanceGate_HonorsZeroRerankScoreWhenApplied(t *testing.T) {
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		{ChunkText: "text", Title: "title", Score: 0, ScoreKind: domain.ScoreKindRerank, RerankScore: 0, RerankApplied: true, ChunkID: uuid.New()},
	}
	assert.Equal(t, QualityInsufficient, gate.Evaluate(contexts))
}

func TestRelevanceGate_ExactThreshold(t *testing.T) {
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{makeCtx("Exact", 0.5)}
	assert.Equal(t, QualityGood, gate.Evaluate(contexts))

	contexts2 := []ContextItem{makeCtx("Exact marginal", 0.25)}
	assert.Equal(t, QualityMarginal, gate.Evaluate(contexts2))
}

func TestRelevanceGate_DriftedBaseline_OffTopicHits_Insufficient(t *testing.T) {
	// Regression: the known failure shape where completely off-topic
	// articles were retrieved for a current-events query. Cross-encoder
	// scores for irrelevant content should be well below 0.25.
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		makeCtx("Orchard Gardening Weekly", 0.03),
		makeCtx("Example Protocol Digest", 0.02),
	}
	assert.Equal(t, QualityInsufficient, gate.Evaluate(contexts))
}

func TestRelevanceGate_RRFScore_IsNotJudgedAgainstRerankerThresholds(t *testing.T) {
	// The thresholds here are calibrated against cross-encoder output on a
	// 0..1 scale. When the reranker does not run — it is disabled, it timed
	// out, or vector search returned nothing so the pipeline degraded to
	// BM25 — ContextItem.Score still holds an RRF score, which for k=60 sits
	// around 0.016–0.033 no matter how relevant the hit is. Judging that
	// against a 0.25 marginal threshold rejects every retrieval, and the
	// caller turns Insufficient into a hard stop: Augur then answers nothing
	// at all. With no reranker score to read, the gate has no evidence the
	// context is good and none that it is bad, so it must not claim it is bad.
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		{ChunkText: "Example Systems recall coverage", Title: "Example Systems escalates recalls", Score: 0.033, ChunkID: uuid.New()},
	}
	assert.Equal(t, QualityMarginal, gate.Evaluate(contexts))
}

func TestRelevanceGate_BM25ScoreKind_IsNeverJudgedGood(t *testing.T) {
	// The 2026-04-11 regression in one assertion: an embedder outage degrades
	// retrieval to BM25, whose scores have no relation to the cross-encoder
	// scale the thresholds were calibrated on. A gate that reads the number
	// without reading its space either condemns every hit (0.25 against an RRF
	// score) or blesses every hit (0.5 against a raw BM25 score). Neither is a
	// judgement, so the gate must decline to make one.
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		{
			ChunkText:   "Example Systems recall coverage",
			Title:       "Example Systems escalates recalls",
			Score:       12.5,
			ScoreKind:   domain.ScoreKindBM25,
			RerankScore: 12.5,
			// The flag says a score exists; the space says it is not one the
			// thresholds can read. The space wins.
			RerankApplied: true,
			ChunkID:       uuid.New(),
		},
	}
	assert.Equal(t, QualityMarginal, gate.Evaluate(contexts))
}

func TestRelevanceGate_RRFScoreKind_IsNeverJudgedInsufficient(t *testing.T) {
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		{
			ChunkText:     "Example Systems recall coverage",
			Title:         "Example Systems escalates recalls",
			Score:         0.033,
			ScoreKind:     domain.ScoreKindRRF,
			RerankScore:   0.033,
			RerankApplied: true,
			ChunkID:       uuid.New(),
		},
	}
	assert.Equal(t, QualityMarginal, gate.Evaluate(contexts))
}
