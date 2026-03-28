package usecase

import (
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

func newTestPlanner() *ConversationPlanner {
	classifier := NewQueryClassifier(nil, 0)
	return NewConversationPlanner(classifier)
}

func articleScopedState() *domain.ConversationState {
	return &domain.ConversationState{
		ThreadID:            "thread-1",
		Mode:                domain.ModeArticleScoped,
		CurrentArticleID:    "article-123",
		CurrentArticleTitle: "Test Article",
		LastAnswerScope:     domain.ScopeSummary,
		TurnCount:           1,
		FocusEntities:       []string{"Iran", "protests", "US"},
		FocusClaims:         []string{"military action risk increased"},
		TopicConfidence:     0.8,
	}
}

// --- Reference Resolution Tests ---

func TestPlanner_AmbiguousDetail_WithState(t *testing.T) {
	planner := newTestPlanner()
	state := articleScopedState()
	intent := QueryIntent{
		IntentType:   IntentArticleScoped,
		ArticleID:    "article-123",
		UserQuestion: "もっと詳しく",
	}

	got := planner.Plan("もっと詳しく", intent, state, nil)

	assert.Equal(t, domain.OpDetail, got.Operation)
	assert.Equal(t, domain.PolicyArticleOnly, got.RetrievalPolicy)
	assert.False(t, got.NeedsClarification)
}

func TestPlanner_AmbiguousDetail_WithoutState(t *testing.T) {
	planner := newTestPlanner()
	intent := QueryIntent{
		IntentType:   IntentGeneral,
		UserQuestion: "もっと詳しく",
	}

	got := planner.Plan("もっと詳しく", intent, nil, nil)

	assert.True(t, got.NeedsClarification)
	assert.Equal(t, domain.OpClarify, got.Operation)
	assert.Equal(t, domain.PolicyNoRetrieval, got.RetrievalPolicy)
	assert.NotEmpty(t, got.ClarificationMsg)
}

func TestPlanner_IsItTrue_ResolvesToFactCheck(t *testing.T) {
	planner := newTestPlanner()
	state := articleScopedState()
	intent := QueryIntent{
		IntentType:   IntentArticleScoped,
		ArticleID:    "article-123",
		UserQuestion: "それって本当？",
	}

	got := planner.Plan("それって本当？", intent, state, nil)

	assert.Equal(t, domain.OpFactCheck, got.Operation)
	assert.Equal(t, domain.PolicyArticlePlusGlobal, got.RetrievalPolicy)
	assert.False(t, got.NeedsClarification)
}

func TestPlanner_RelatedArticles_WithArticleScope(t *testing.T) {
	planner := newTestPlanner()
	state := articleScopedState()
	intent := QueryIntent{
		IntentType:    IntentArticleScoped,
		SubIntentType: SubIntentRelatedArticles,
		ArticleID:     "article-123",
		UserQuestion:  "関連する記事はある？",
	}

	got := planner.Plan("関連する記事はある？", intent, state, nil)

	assert.Equal(t, domain.OpRelatedArticles, got.Operation)
	assert.Equal(t, domain.PolicyToolOnly, got.RetrievalPolicy)
	assert.False(t, got.NeedsClarification)
}

func TestPlanner_TopicShift(t *testing.T) {
	planner := newTestPlanner()
	state := articleScopedState()
	intent := QueryIntent{
		IntentType:   IntentGeneral,
		UserQuestion: "ここからは別件だけど、最新の原油価格は？",
	}

	got := planner.Plan("ここからは別件だけど、最新の原油価格は？", intent, state, nil)

	assert.Equal(t, domain.OpTopicShift, got.Operation)
	assert.Equal(t, domain.PolicyGlobalOnly, got.RetrievalPolicy)
	assert.Equal(t, domain.ScopeDrop, got.ArticleScopeAction)
}

func TestPlanner_DifferentPerspective(t *testing.T) {
	planner := newTestPlanner()
	state := articleScopedState()
	intent := QueryIntent{
		IntentType:   IntentArticleScoped,
		ArticleID:    "article-123",
		UserQuestion: "別の観点では？",
	}

	got := planner.Plan("別の観点では？", intent, state, nil)

	assert.Equal(t, domain.OpCritique, got.Operation)
	assert.False(t, got.NeedsClarification)
}

// --- SubIntent Passthrough Tests ---

func TestPlanner_SubIntentPassthrough(t *testing.T) {
	tests := []struct {
		name          string
		subIntent     SubIntentType
		wantOperation domain.PlannerOperation
		wantPolicy    domain.RetrievalPolicy
	}{
		{"detail", SubIntentDetail, domain.OpDetail, domain.PolicyArticleOnly},
		{"evidence", SubIntentEvidence, domain.OpEvidence, domain.PolicyArticleOnly},
		{"related_articles", SubIntentRelatedArticles, domain.OpRelatedArticles, domain.PolicyToolOnly},
		{"critique", SubIntentCritique, domain.OpCritique, domain.PolicyArticleOnly},
		{"opinion", SubIntentOpinion, domain.OpOpinion, domain.PolicyArticleOnly},
		{"implication", SubIntentImplication, domain.OpImplication, domain.PolicyArticlePlusGlobal},
		{"summary_refresh", SubIntentSummaryRefresh, domain.OpSummaryRefresh, domain.PolicyArticleOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planner := newTestPlanner()
			intent := QueryIntent{
				IntentType:    IntentArticleScoped,
				SubIntentType: tt.subIntent,
				ArticleID:     "article-123",
				UserQuestion:  "test query",
			}

			got := planner.Plan("test query", intent, articleScopedState(), nil)

			assert.Equal(t, tt.wantOperation, got.Operation)
			assert.Equal(t, tt.wantPolicy, got.RetrievalPolicy)
		})
	}
}

// --- Retrieval Policy Matrix Tests ---

func TestPlanner_RetrievalPolicyMatrix(t *testing.T) {
	tests := []struct {
		name       string
		intentType IntentType
		subIntent  SubIntentType
		articleID  string
		wantPolicy domain.RetrievalPolicy
	}{
		{"general", IntentGeneral, SubIntentNone, "", domain.PolicyGlobalOnly},
		{"comparison", IntentComparison, SubIntentNone, "", domain.PolicyArticlePlusGlobal},
		{"temporal", IntentTemporal, SubIntentNone, "", domain.PolicyGlobalOnly},
		{"fact_check", IntentFactCheck, SubIntentNone, "", domain.PolicyArticlePlusGlobal},
		{"topic_deep_dive", IntentTopicDeepDive, SubIntentNone, "", domain.PolicyGlobalOnly},
		{"article_detail", IntentArticleScoped, SubIntentDetail, "art-1", domain.PolicyArticleOnly},
		{"article_evidence", IntentArticleScoped, SubIntentEvidence, "art-1", domain.PolicyArticleOnly},
		{"article_related", IntentArticleScoped, SubIntentRelatedArticles, "art-1", domain.PolicyToolOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planner := newTestPlanner()
			intent := QueryIntent{
				IntentType:    tt.intentType,
				SubIntentType: tt.subIntent,
				ArticleID:     tt.articleID,
				UserQuestion:  "test query",
			}
			state := articleScopedState()
			if tt.articleID == "" {
				state = nil
			}

			got := planner.Plan("test query", intent, state, nil)

			assert.Equal(t, tt.wantPolicy, got.RetrievalPolicy)
		})
	}
}

// --- Clarification Tests ---

func TestPlanner_ClarificationMessage_IncludesEntities(t *testing.T) {
	planner := newTestPlanner()
	state := &domain.ConversationState{
		ThreadID:        "thread-1",
		Mode:            domain.ModeArticleScoped,
		FocusEntities:   []string{"Iran", "protests", "US"},
		TopicConfidence: 0.3, // Low confidence triggers clarification
	}
	intent := QueryIntent{
		IntentType:   IntentArticleScoped,
		ArticleID:    "article-123",
		UserQuestion: "もっと詳しく",
	}

	got := planner.Plan("もっと詳しく", intent, state, nil)

	assert.True(t, got.NeedsClarification)
	assert.Contains(t, got.ClarificationMsg, "Iran")
}

func TestPlanner_AmbiguousWithHighConfidence_NosClarification(t *testing.T) {
	planner := newTestPlanner()
	state := articleScopedState() // TopicConfidence: 0.8
	intent := QueryIntent{
		IntentType:   IntentArticleScoped,
		ArticleID:    "article-123",
		UserQuestion: "もっと詳しく",
	}

	got := planner.Plan("もっと詳しく", intent, state, nil)

	assert.False(t, got.NeedsClarification)
	assert.Equal(t, domain.OpDetail, got.Operation)
}

// --- Confidence Tests ---

func TestPlanner_Confidence_HighForExplicitSubIntent(t *testing.T) {
	planner := newTestPlanner()
	intent := QueryIntent{
		IntentType:    IntentArticleScoped,
		SubIntentType: SubIntentDetail,
		ArticleID:     "article-123",
		UserQuestion:  "技術的な詳細をもっと教えて",
	}

	got := planner.Plan("技術的な詳細をもっと教えて", intent, articleScopedState(), nil)

	assert.GreaterOrEqual(t, got.Confidence, 0.8)
}

func TestPlanner_Confidence_LowerForAmbiguous(t *testing.T) {
	planner := newTestPlanner()
	state := articleScopedState()
	intent := QueryIntent{
		IntentType:   IntentArticleScoped,
		ArticleID:    "article-123",
		UserQuestion: "もっと詳しく",
	}

	got := planner.Plan("もっと詳しく", intent, state, nil)

	assert.Less(t, got.Confidence, 0.9)
}

// --- Article Scope Action Tests ---

func TestPlanner_ArticleScopeAction_Keep(t *testing.T) {
	planner := newTestPlanner()
	intent := QueryIntent{
		IntentType:    IntentArticleScoped,
		SubIntentType: SubIntentDetail,
		ArticleID:     "article-123",
		UserQuestion:  "技術的な詳細",
	}

	got := planner.Plan("技術的な詳細", intent, articleScopedState(), nil)

	assert.Equal(t, domain.ScopeKeep, got.ArticleScopeAction)
}

// --- Fix 1: Ambiguous follow-up over-classification ---

func TestIsAmbiguousFollowUp_TopicPlusPattern_NotAmbiguous(t *testing.T) {
	// "PyO3について詳しく教えて" contains a specific topic — not ambiguous.
	assert.False(t, isAmbiguousFollowUp("PyO3について詳しく教えて"),
		"query with specific topic + pattern should not be ambiguous")
	assert.False(t, isAmbiguousFollowUp("そのリスクについて詳しく教えて"),
		"query with demonstrative + topic + pattern should not be ambiguous")
	assert.False(t, isAmbiguousFollowUp("Rustのメモリ管理についてもっと詳しく"),
		"query with specific entity + pattern should not be ambiguous")
}

func TestIsAmbiguousFollowUp_PatternOnly_IsAmbiguous(t *testing.T) {
	// Pattern-only queries without substantive content remain ambiguous.
	assert.True(t, isAmbiguousFollowUp("もっと詳しく"),
		"bare pattern should be ambiguous")
	assert.True(t, isAmbiguousFollowUp("詳しく教えて"),
		"bare pattern should be ambiguous")
	assert.True(t, isAmbiguousFollowUp("それって本当？"),
		"bare pattern should be ambiguous")
	assert.True(t, isAmbiguousFollowUp("別の観点では？"),
		"bare pattern should be ambiguous")
}

func TestPlanner_NilState_WithHistory_FallsBackToDetail(t *testing.T) {
	// When state is nil but conversation history exists, the planner should
	// NOT demand clarification — it should fall back to OpDetail.
	planner := newTestPlanner()
	intent := QueryIntent{
		IntentType:   IntentGeneral,
		UserQuestion: "もっと詳しく",
	}
	history := []domain.Message{
		{Role: "user", Content: "PyO3について教えて"},
		{Role: "assistant", Content: "PyO3はPythonとRustの橋渡しライブラリです。"},
	}

	got := planner.Plan("もっと詳しく", intent, nil, history)

	assert.Equal(t, domain.OpDetail, got.Operation,
		"nil state + non-empty history should fall back to OpDetail, not OpClarify")
	assert.False(t, got.NeedsClarification,
		"should not demand clarification when history provides context")
}

func TestPlanner_ArticleScopeAction_Drop(t *testing.T) {
	planner := newTestPlanner()
	intent := QueryIntent{
		IntentType:   IntentGeneral,
		UserQuestion: "ここからは別件だけど",
	}

	got := planner.Plan("ここからは別件だけど", intent, articleScopedState(), nil)

	assert.Equal(t, domain.ScopeDrop, got.ArticleScopeAction)
}
