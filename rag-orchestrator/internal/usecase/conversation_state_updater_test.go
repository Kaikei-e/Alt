package usecase

import (
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestDeriveStateUpdate_InitialArticleScoped(t *testing.T) {
	intent := QueryIntent{
		IntentType:   IntentArticleScoped,
		ArticleID:    "article-123",
		ArticleTitle: "Test Article Title",
		UserQuestion: "技術的な詳細をもっと教えて",
	}
	plannerOutput := &domain.PlannerOutput{
		Operation:       domain.OpDetail,
		RetrievalPolicy: domain.PolicyArticleOnly,
		Confidence:      0.9,
	}
	answerOutput := &AnswerWithRAGOutput{
		Answer: "技術的な詳細について...",
		Citations: []Citation{
			{ChunkID: "chunk-1"},
			{ChunkID: "chunk-2"},
		},
	}

	got := DeriveStateUpdate(nil, "thread-1", intent, plannerOutput, answerOutput)

	assert.Equal(t, "thread-1", got.ThreadID)
	assert.Equal(t, domain.ModeArticleScoped, got.Mode)
	assert.Equal(t, "article-123", got.CurrentArticleID)
	assert.Equal(t, "Test Article Title", got.CurrentArticleTitle)
	assert.Equal(t, domain.ScopeDetail, got.LastAnswerScope)
	assert.Equal(t, []string{"chunk-1", "chunk-2"}, got.LastCitations)
	assert.Equal(t, 1, got.TurnCount)
}

func TestDeriveStateUpdate_CarryForwardArticleScope(t *testing.T) {
	prev := &domain.ConversationState{
		ThreadID:            "thread-1",
		Mode:                domain.ModeArticleScoped,
		CurrentArticleID:    "article-123",
		CurrentArticleTitle: "Test Article",
		TurnCount:           1,
		FocusEntities:       []string{"Iran", "protests"},
	}
	// Follow-up question without article metadata — article scope should carry forward
	intent := QueryIntent{
		IntentType:   IntentArticleScoped,
		ArticleID:    "article-123",
		UserQuestion: "この記事の根拠は？",
	}
	plannerOutput := &domain.PlannerOutput{
		Operation:       domain.OpEvidence,
		RetrievalPolicy: domain.PolicyArticleOnly,
		Confidence:      0.85,
	}
	answerOutput := &AnswerWithRAGOutput{
		Answer:    "根拠として...",
		Citations: []Citation{{ChunkID: "chunk-3"}},
	}

	got := DeriveStateUpdate(prev, "thread-1", intent, plannerOutput, answerOutput)

	assert.Equal(t, "thread-1", got.ThreadID)
	assert.Equal(t, domain.ModeArticleScoped, got.Mode)
	assert.Equal(t, "article-123", got.CurrentArticleID)
	assert.Equal(t, domain.ScopeEvidence, got.LastAnswerScope)
	assert.Equal(t, []string{"chunk-3"}, got.LastCitations)
	assert.Equal(t, 2, got.TurnCount)
}

func TestDeriveStateUpdate_TopicShift(t *testing.T) {
	prev := &domain.ConversationState{
		ThreadID:         "thread-1",
		Mode:             domain.ModeArticleScoped,
		CurrentArticleID: "article-123",
		TurnCount:        3,
		FocusEntities:    []string{"Iran", "protests"},
	}
	intent := QueryIntent{
		IntentType:   IntentGeneral,
		UserQuestion: "最新の原油価格は？",
	}
	plannerOutput := &domain.PlannerOutput{
		Operation:       domain.OpTopicShift,
		RetrievalPolicy: domain.PolicyGlobalOnly,
		Confidence:      0.95,
	}
	answerOutput := &AnswerWithRAGOutput{
		Answer: "原油価格について...",
	}

	got := DeriveStateUpdate(prev, "thread-1", intent, plannerOutput, answerOutput)

	assert.Equal(t, domain.ModeOpenTopic, got.Mode)
	assert.Empty(t, got.CurrentArticleID)
	assert.Empty(t, got.FocusEntities, "topic shift should reset focus entities")
	assert.Equal(t, 4, got.TurnCount)
}

func TestDeriveStateUpdate_GeneralQuery(t *testing.T) {
	intent := QueryIntent{
		IntentType:   IntentGeneral,
		UserQuestion: "AIの最新動向は？",
	}
	plannerOutput := &domain.PlannerOutput{
		Operation:       domain.OpGeneral,
		RetrievalPolicy: domain.PolicyGlobalOnly,
		Confidence:      0.8,
	}
	answerOutput := &AnswerWithRAGOutput{
		Answer: "AI動向について...",
	}

	got := DeriveStateUpdate(nil, "thread-2", intent, plannerOutput, answerOutput)

	assert.Equal(t, "thread-2", got.ThreadID)
	assert.Equal(t, domain.ModeOpenTopic, got.Mode)
	assert.Empty(t, got.CurrentArticleID)
	assert.Equal(t, domain.ScopeSummary, got.LastAnswerScope)
	assert.Equal(t, 1, got.TurnCount)
}

func TestDeriveStateUpdate_OperationToScopeMapping(t *testing.T) {
	tests := []struct {
		name      string
		operation domain.PlannerOperation
		wantScope domain.AnswerScope
	}{
		{"detail", domain.OpDetail, domain.ScopeDetail},
		{"evidence", domain.OpEvidence, domain.ScopeEvidence},
		{"related_articles", domain.OpRelatedArticles, domain.ScopeRelatedArticles},
		{"critique", domain.OpCritique, domain.ScopeCritique},
		{"opinion", domain.OpOpinion, domain.ScopeOpinion},
		{"implication", domain.OpImplication, domain.ScopeImplication},
		{"general", domain.OpGeneral, domain.ScopeSummary},
		{"fact_check", domain.OpFactCheck, domain.ScopeSummary},
		{"summary_refresh", domain.OpSummaryRefresh, domain.ScopeSummary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := QueryIntent{IntentType: IntentGeneral, UserQuestion: "test"}
			plan := &domain.PlannerOutput{Operation: tt.operation, Confidence: 0.8}
			answer := &AnswerWithRAGOutput{Answer: "test answer"}

			got := DeriveStateUpdate(nil, "thread-1", intent, plan, answer)
			assert.Equal(t, tt.wantScope, got.LastAnswerScope)
		})
	}
}

func TestDeriveStateUpdate_CitationsExtracted(t *testing.T) {
	intent := QueryIntent{IntentType: IntentGeneral, UserQuestion: "test"}
	plan := &domain.PlannerOutput{Operation: domain.OpGeneral, Confidence: 0.8}
	answer := &AnswerWithRAGOutput{
		Answer: "test",
		Citations: []Citation{
			{ChunkID: "chunk-a"},
			{ChunkID: "chunk-b"},
			{ChunkID: "chunk-c"},
		},
	}

	got := DeriveStateUpdate(nil, "thread-1", intent, plan, answer)
	assert.Equal(t, []string{"chunk-a", "chunk-b", "chunk-c"}, got.LastCitations)
}

func TestDeriveStateUpdate_NilPlannerOutput(t *testing.T) {
	intent := QueryIntent{
		IntentType:   IntentArticleScoped,
		ArticleID:    "article-123",
		ArticleTitle: "Title",
		UserQuestion: "test",
	}
	answer := &AnswerWithRAGOutput{Answer: "test"}

	got := DeriveStateUpdate(nil, "thread-1", intent, nil, answer)

	assert.Equal(t, domain.ModeArticleScoped, got.Mode)
	assert.Equal(t, "article-123", got.CurrentArticleID)
	assert.Equal(t, domain.ScopeSummary, got.LastAnswerScope)
}
