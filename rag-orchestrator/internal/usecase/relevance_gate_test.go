package usecase

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func makeCtx(title string, rerankScore float32) ContextItem {
	return ContextItem{
		ChunkText:     "chunk text for " + title,
		Title:         title,
		Score:         0.5,
		RerankScore:   rerankScore,
		RerankApplied: true,
		ChunkID:       uuid.New(),
	}
}

func TestRelevanceGate_Good(t *testing.T) {
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		makeCtx("Iran escalates attacks", 0.82),
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
		makeCtx("Asset Tokenization", 0.10),
		makeCtx("LibreFang", 0.05),
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
		{ChunkText: "text", Title: "title", Score: 0.9, RerankScore: 0, RerankApplied: true, ChunkID: uuid.New()},
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

func TestRelevanceGate_IranBaseline_AssetTokenization_Insufficient(t *testing.T) {
	// Regression: the known failure case where "Asset Tokenization" was
	// retrieved for an Iran oil crisis query. Cross-encoder scores for
	// completely irrelevant content should be well below 0.25.
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		makeCtx("Asset Tokenization", 0.03),
		makeCtx("LibreFang", 0.02),
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
		{ChunkText: "Iran oil crisis coverage", Title: "Iran escalates attacks", Score: 0.033, ChunkID: uuid.New()},
	}
	assert.Equal(t, QualityMarginal, gate.Evaluate(contexts))
}
