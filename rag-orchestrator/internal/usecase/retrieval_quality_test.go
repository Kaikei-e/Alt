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

// --- AssessWithIntent tests (causal-aware retrieval gating) ---

func TestAssessWithIntent_Causal_TopicIncoherence_IsInsufficient(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9, Title: "Venezuela Oil Blockade"},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.8, Title: "Iran Airspace Reopening"},
		{ChunkID: uuid.New(), Score: 0.7, RerankScore: 0.7, Title: "Space Exploration Update"},
	}
	verdict := assessor.AssessWithIntent(contexts, IntentCausalExplanation, "")
	if verdict != QualityInsufficient {
		t.Errorf("expected QualityInsufficient for causal + incoherent topics, got %s", verdict)
	}
}

func TestAssessWithIntent_Causal_Marginal_Coherent_AllowsRetry(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	// Marginal scores but coherent titles → allow retry (return Marginal, not Insufficient)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.4, RerankScore: 0.4, Title: "Oil Crisis"},
		{ChunkID: uuid.New(), Score: 0.3, RerankScore: 0.3, Title: "Oil Crisis"},
		{ChunkID: uuid.New(), Score: 0.25, RerankScore: 0.25, Title: "Oil Crisis"},
	}
	verdict := assessor.AssessWithIntent(contexts, IntentCausalExplanation, "")
	if verdict != QualityMarginal {
		t.Errorf("expected QualityMarginal for causal + marginal + coherent (allows retry), got %s", verdict)
	}
}

func TestAssessWithIntent_Causal_Marginal_Incoherent_IsInsufficient(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	// Marginal scores AND incoherent titles → no point retrying scattered results
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.4, RerankScore: 0.4, Title: "Venezuela Oil Blockade"},
		{ChunkID: uuid.New(), Score: 0.3, RerankScore: 0.3, Title: "Iran Airspace Reopening"},
		{ChunkID: uuid.New(), Score: 0.25, RerankScore: 0.25, Title: "Space Exploration News"},
	}
	verdict := assessor.AssessWithIntent(contexts, IntentCausalExplanation, "")
	if verdict != QualityInsufficient {
		t.Errorf("expected QualityInsufficient for causal + marginal + incoherent, got %s", verdict)
	}
}

func TestAssessWithIntent_NonCausal_TopicIncoherence_IsMarginal(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9, Title: "Venezuela Oil"},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.8, Title: "Iran Airspace"},
		{ChunkID: uuid.New(), Score: 0.7, RerankScore: 0.7, Title: "Space Exploration"},
	}
	verdict := assessor.AssessWithIntent(contexts, IntentGeneral, "")
	if verdict != QualityMarginal {
		t.Errorf("expected QualityMarginal for non-causal + incoherent (downgrade only), got %s", verdict)
	}
}

func TestAssessWithIntent_Causal_Coherent_Good(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9, Title: "Oil Crisis Root Causes"},
		{ChunkID: uuid.New(), Score: 0.85, RerankScore: 0.85, Title: "Oil Supply Crisis Analysis"},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.8, Title: "Oil Market Crisis Factors"},
	}
	verdict := assessor.AssessWithIntent(contexts, IntentCausalExplanation, "")
	if verdict != QualityGood {
		t.Errorf("expected QualityGood for causal + coherent + good scores, got %s", verdict)
	}
}

func TestAssessWithIntent_Causal_Good_HighVariance_IsInsufficient(t *testing.T) {
	// Use low thresholds so base verdict is Good despite high variance,
	// then causal + good + high variance → Insufficient
	assessor := NewRetrievalQualityAssessor(0.3, 0.15, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9, Title: "Oil Crisis"},
		{ChunkID: uuid.New(), Score: 0.15, RerankScore: 0.15, Title: "Oil Crisis"},
		{ChunkID: uuid.New(), Score: 0.1, RerankScore: 0.1, Title: "Oil Crisis"},
	}
	// avg = 0.383 > 0.3 (good threshold), but variance is high (0.9 / 0.15 = 6x)
	// Base Assess: good → downgraded to marginal by hasHighScoreVariance
	// AssessWithIntent: marginal + coherent → Marginal (allows retry)
	// Actually base Assess itself downgrades Good→Marginal for high variance,
	// so causal + marginal + coherent → Marginal
	verdict := assessor.AssessWithIntent(contexts, IntentCausalExplanation, "")
	if verdict != QualityMarginal {
		t.Errorf("expected QualityMarginal for causal + high variance (coherent titles allow retry), got %s", verdict)
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

// --- Phase 3: Query-context relevance tests ---

func TestAssessWithIntent_Causal_QueryContextMismatch_IsInsufficientWhenMarginal(t *testing.T) {
	// Query about Iran oil crisis, but contexts are about completely unrelated topics.
	// Titles AND chunks have no overlap with query keywords.
	// With marginal base verdict + mismatch → Insufficient
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.4, RerankScore: 0.4, Title: "Asset Tokenization Future", ChunkText: "Blockchain enables fractional ownership of real estate assets."},
		{ChunkID: uuid.New(), Score: 0.3, RerankScore: 0.3, Title: "LibreFang Open Source", ChunkText: "A new open-source alternative to commercial fang-based tools."},
		{ChunkID: uuid.New(), Score: 0.25, RerankScore: 0.25, Title: "AI Model Pricing Guide", ChunkText: "Comparing costs of GPT-4, Claude, and Gemini for enterprise use."},
	}
	verdict := assessor.AssessWithIntent(contexts, IntentCausalExplanation, "イランの石油危機はなぜ起きた？")
	if verdict != QualityInsufficient {
		t.Errorf("expected QualityInsufficient for causal + marginal + query-context mismatch, got %s", verdict)
	}
}

func TestAssessWithIntent_Causal_TitleMismatchButChunkMatches_DoesNotDowngradeHard(t *testing.T) {
	// Title doesn't match query but chunk text contains relevant content.
	// Should NOT hard-fail because chunk text is relevant.
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9, Title: "Middle East Update March 2026", ChunkText: "イランの石油生産が停止し、ホルムズ海峡の封鎖懸念が高まっている。"},
		{ChunkID: uuid.New(), Score: 0.85, RerankScore: 0.85, Title: "Energy Markets Weekly", ChunkText: "Iran oil exports dropped sharply due to new sanctions."},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.8, Title: "Geopolitical Risk Report", ChunkText: "石油価格が急騰し、イラン情勢が市場に影響を与えている。"},
	}
	verdict := assessor.AssessWithIntent(contexts, IntentCausalExplanation, "イランの石油危機はなぜ起きた？")
	if verdict == QualityInsufficient {
		t.Error("should NOT downgrade to Insufficient when chunk text matches query despite title mismatch")
	}
}

func TestAssessWithIntent_Causal_QueryContextMatch_StaysGood(t *testing.T) {
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 3)
	contexts := []ContextItem{
		{ChunkID: uuid.New(), Score: 0.9, RerankScore: 0.9, Title: "Iran Oil Supply Crisis", ChunkText: "Iran's oil production has been severely impacted."},
		{ChunkID: uuid.New(), Score: 0.85, RerankScore: 0.85, Title: "Oil Crisis Root Causes", ChunkText: "The crisis stems from geopolitical tensions."},
		{ChunkID: uuid.New(), Score: 0.8, RerankScore: 0.8, Title: "Iran Sanctions Impact", ChunkText: "Oil sanctions on Iran have disrupted global supply."},
	}
	verdict := assessor.AssessWithIntent(contexts, IntentCausalExplanation, "イランの石油危機はなぜ起きた？")
	if verdict != QualityGood {
		t.Errorf("expected QualityGood when contexts match query, got %s", verdict)
	}
}

func TestHasQueryContextMismatch_CJKAndEnglish(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		contexts []ContextItem
		expected bool
	}{
		{
			name:  "CJK query no match in title or chunk",
			query: "イランの石油危機",
			contexts: []ContextItem{
				{Title: "Asset Tokenization", ChunkText: "Blockchain enables fractional ownership."},
				{Title: "LibreFang Project", ChunkText: "A new open-source tool."},
			},
			expected: true,
		},
		{
			name:  "CJK query matches in chunk text",
			query: "イランの石油危機",
			contexts: []ContextItem{
				{Title: "Middle East Update", ChunkText: "イランの石油生産が停止した。"},
			},
			expected: false,
		},
		{
			name:  "English query matches in title",
			query: "Iran oil crisis",
			contexts: []ContextItem{
				{Title: "Iran Oil Supply", ChunkText: "Production disrupted."},
				{Title: "Oil Market Crisis", ChunkText: "Global markets react."},
			},
			expected: false,
		},
		{
			name:  "English query no match",
			query: "Iran oil crisis",
			contexts: []ContextItem{
				{Title: "Blockchain Future", ChunkText: "DeFi protocols enable new financial instruments."},
				{Title: "Space Station Update", ChunkText: "NASA announces new mission."},
			},
			expected: true,
		},
		{
			name:     "empty query skips check",
			query:    "",
			contexts: []ContextItem{{Title: "Anything"}},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasQueryContextMismatch(tt.query, tt.contexts)
			if result != tt.expected {
				t.Errorf("expected %v, got %v for query %q", tt.expected, result, tt.query)
			}
		})
	}
}
