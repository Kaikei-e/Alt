package usecase

import (
	"testing"

	"github.com/google/uuid"
)

func TestAssess_AllHighScores_ReturnsGood(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.8},
		{ChunkID: uuid.New(), Score: 0.7, RerankScore: 0.7},
	}
	verdict := assessor.Assess(contexts)
	if verdict != QualityGood {
		t.Errorf("expected QualityGood, got %s", verdict)
	}
}

func TestAssess_MediumScores_ReturnsMarginal(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.4, RerankScore: 0.4},
		{ChunkID: uuid.New(), Score: 0.3, RerankScore: 0.3},
		{ChunkID: uuid.New(), Score: 0.25, RerankScore: 0.25},
	}
	verdict := assessor.Assess(contexts)
	if verdict != QualityMarginal {
		t.Errorf("expected QualityMarginal, got %s", verdict)
	}
}

func TestAssess_LowScores_ReturnsInsufficient(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.1, RerankScore: 0.1},
		{ChunkID: uuid.New(), Score: 0.05, RerankScore: 0.05},
		{ChunkID: uuid.New(), Score: 0.02, RerankScore: 0.02},
	}
	verdict := assessor.Assess(contexts)
	if verdict != QualityInsufficient {
		t.Errorf("expected QualityInsufficient, got %s", verdict)
	}
}

func TestAssess_EmptyContexts_ReturnsInsufficient(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	verdict := assessor.Assess(nil)
	if verdict != QualityInsufficient {
		t.Errorf("expected QualityInsufficient, got %s", verdict)
	}
}

func TestAssess_FewerThanMinContexts_ReturnsInsufficient(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.8},
	}
	verdict := assessor.Assess(contexts)
	if verdict != QualityInsufficient {
		t.Errorf("expected QualityInsufficient, got %s", verdict)
	}
}

func TestAssess_UsesRerankScoreWhenAvailable(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	// Score (RRF) is high but RerankScore is low — should use RerankScore
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.1},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.05},
		{ChunkID: uuid.New(), Score: 0.7, RerankScore: 0.02},
	}
	verdict := assessor.Assess(contexts)
	if verdict != QualityInsufficient {
		t.Errorf("expected QualityInsufficient (using RerankScore), got %s", verdict)
	}
}

func TestAssess_FallsBackToScoreWhenRerankScoreZero(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	// RerankScore is 0 (reranking disabled) — fall back to Score
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0},
		{ChunkID: uuid.New(), Score: 0.7, RerankScore: 0},
	}
	verdict := assessor.Assess(contexts)
	if verdict != QualityGood {
		t.Errorf("expected QualityGood (fallback to Score), got %s", verdict)
	}
}

func TestAssess_ExactlyAtGoodThreshold(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.5, RerankScore: 0.5},
		{ChunkID: uuid.New(), Score: 0.5, RerankScore: 0.5},
		{ChunkID: uuid.New(), Score: 0.5, RerankScore: 0.5},
	}
	verdict := assessor.Assess(contexts)
	if verdict != QualityGood {
		t.Errorf("expected QualityGood at exact threshold, got %s", verdict)
	}
}

func TestAssess_TopicIncoherence_DifferentTitles_Downgrades(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	// High scores but contexts come from completely different articles — topic incoherence
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9, Title: "Iran Protests"},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.8, Title: "Cooking Recipes"},
		{ChunkID: uuid.New(), Score: 0.7, RerankScore: 0.7, Title: "Space Exploration"},
	}
	verdict := assessor.Assess(contexts)
	if verdict == QualityGood {
		t.Error("expected downgrade from QualityGood when top contexts have unrelated titles")
	}
}

func TestAssess_TopicCoherence_SameTitle_NoDowngrade(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9, Title: "Iran Protests"},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.8, Title: "Iran Protests"},
		{ChunkID: uuid.New(), Score: 0.7, RerankScore: 0.7, Title: "Iran and Middle East"},
	}
	verdict := assessor.Assess(contexts)
	if verdict != QualityGood {
		t.Errorf("expected QualityGood for coherent contexts, got %s", verdict)
	}
}

func TestAssess_ScoreVariance_HighSpread_Downgrades(t *testing.T) {
	// Top-1 is very high, but remaining are very low = "one hit + noise" pattern
	// Even with low thresholds that would pass by avg alone, variance should downgrade
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.95, RerankScore: 0.95, Title: "Topic A"},
		{ChunkID: uuid.New(), Score: 0.1, RerankScore: 0.1, Title: "Topic A"},
		{ChunkID: uuid.New(), Score: 0.08, RerankScore: 0.08, Title: "Topic A"},
	}
	assessor := NewRetrievalQualityAssessor(0.3, 0.15, 3)
	verdict := assessor.Assess(contexts)
	if verdict == QualityGood {
		t.Error("expected downgrade when score variance is very high (one hit + noise)")
	}
}

func TestAssess_MoreThanThreeContexts_UsesTopThree(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.8},
		{ChunkID: uuid.New(), Score: 0.7, RerankScore: 0.7},
		{ChunkID: uuid.New(), Score: 0.01, RerankScore: 0.01}, // low score, but not in top-3
		{ChunkID: uuid.New(), Score: 0.01, RerankScore: 0.01},
	}
	verdict := assessor.Assess(contexts)
	if verdict != QualityGood {
		t.Errorf("expected QualityGood (top-3 are high), got %s", verdict)
	}
}
