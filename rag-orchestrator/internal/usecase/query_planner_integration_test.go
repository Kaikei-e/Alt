package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockQueryPlannerPort implements domain.QueryPlannerPort for testing.
type mockQueryPlannerPort struct {
	mock.Mock
}

func (m *mockQueryPlannerPort) PlanQuery(ctx context.Context, input domain.QueryPlannerInput) (*domain.QueryPlan, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.QueryPlan), args.Error(1)
}

func TestExecute_WithQueryPlanner_UsesResolvedQuery(t *testing.T) {
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	mockQP := new(mockQueryPlannerPort)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// QueryPlanner returns a plan with resolved_query
	mockQP.On("PlanQuery", mock.Anything, mock.MatchedBy(func(input domain.QueryPlannerInput) bool {
		return input.Query == "イランの石油危機はなぜ起きた？"
	})).Return(&domain.QueryPlan{
		ResolvedQuery:   "イランの石油危機が発生した背景と直接的原因",
		SearchQueries:   []string{"イラン 石油危機 原因", "Iran oil crisis causes"},
		Intent:          "causal_explanation",
		RetrievalPolicy: "global_only",
		AnswerFormat:    "causal_analysis",
		ShouldClarify:   false,
		TopicEntities:   []string{"イラン", "石油"},
	}, nil)

	// Retrieve should be called with the RESOLVED query, not the original
	mockRetrieve.On("Execute", mock.Anything, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "イランの石油危機が発生した背景と直接的原因"
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkText: "イランに対する制裁が石油輸出を停止させた",
				Title:     "Iran escalates attacks",
				URL:       "https://example.com/iran",
				Score:     0.85,
				ChunkID:   uuid.New(),
			},
		},
		ExpandedQueries: []string{"イラン 石油危機 原因", "Iran oil crisis causes"},
	}, nil)

	// LLM Chat returns a valid JSON answer
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(&domain.LLMResponse{
		Text: `{"answer":"イランの石油危機は制裁強化が直接的な原因です。[1]","citations":[{"chunk_id":"1","reason":"制裁による影響"}],"fallback":false,"reason":""}`,
	}, nil)

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve,
		usecase.NewXMLPromptBuilder(),
		mockLLM,
		usecase.NewOutputValidator(100),
		7, 512, 6000,
		"v1", "ja",
		testLogger,
		usecase.WithQueryPlanner(mockQP),
	)

	output, err := uc.Execute(context.Background(), usecase.AnswerWithRAGInput{
		Query: "イランの石油危機はなぜ起きた？",
	})

	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.False(t, output.Fallback)
	assert.Contains(t, output.Answer, "制裁")

	// Verify QueryPlanner was called
	mockQP.AssertCalled(t, "PlanQuery", mock.Anything, mock.Anything)

	// Verify retrieval used resolved query
	mockRetrieve.AssertCalled(t, "Execute", mock.Anything, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "イランの石油危機が発生した背景と直接的原因"
	}))
}

func TestExecute_WithQueryPlanner_ClarificationSkipsRetrieval(t *testing.T) {
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	mockQP := new(mockQueryPlannerPort)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	mockQP.On("PlanQuery", mock.Anything, mock.Anything).Return(&domain.QueryPlan{
		ResolvedQuery:    "",
		SearchQueries:    nil,
		Intent:           "general",
		RetrievalPolicy:  "no_retrieval",
		AnswerFormat:     "detail",
		ShouldClarify:    true,
		ClarificationMsg: "何を詳しく知りたいですか？",
		TopicEntities:    nil,
	}, nil)

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve,
		usecase.NewXMLPromptBuilder(),
		mockLLM,
		usecase.NewOutputValidator(100),
		7, 512, 6000,
		"v1", "ja",
		testLogger,
		usecase.WithQueryPlanner(mockQP),
	)

	// Use Stream to test clarification path
	ch := uc.Stream(context.Background(), usecase.AnswerWithRAGInput{
		Query: "もっと詳しく",
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "AIの動向は？"},
			{Role: "assistant", Content: "LLMが進化しています。"},
		},
	})

	var events []usecase.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Should have a clarification event
	hasClarification := false
	for _, ev := range events {
		if ev.Kind == usecase.StreamEventKindClarification {
			hasClarification = true
			clar := ev.Payload.(usecase.StreamClarification)
			assert.Equal(t, "何を詳しく知りたいですか？", clar.Message)
		}
	}
	assert.True(t, hasClarification, "expected clarification event")

	// Retrieve should NOT have been called
	mockRetrieve.AssertNotCalled(t, "Execute", mock.Anything, mock.Anything)
}
