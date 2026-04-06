package usecase

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func makeCtx(title string, rerankScore float32) ContextItem {
	return ContextItem{
		ChunkText:   "chunk text for " + title,
		Title:       title,
		Score:       0.5,
		RerankScore: rerankScore,
		ChunkID:     uuid.New(),
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

func TestRelevanceGate_FallsBackToScore_WhenNoRerankScore(t *testing.T) {
	gate := NewRelevanceGate(0.5, 0.25)
	contexts := []ContextItem{
		{ChunkText: "text", Title: "title", Score: 0.7, RerankScore: 0, ChunkID: uuid.New()},
	}
	assert.Equal(t, QualityGood, gate.Evaluate(contexts))
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
