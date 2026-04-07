package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
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
		return strings.Contains(input.Query, "世界的な物流混乱はなぜ起きた？")
	})).Return(&domain.QueryPlan{
		ResolvedQuery:   "世界的な物流混乱が発生した背景と直接的原因",
		SearchQueries:   []string{"物流混乱 原因", "global supply disruption causes"},
		Intent:          "causal_explanation",
		RetrievalPolicy: "global_only",
		AnswerFormat:    "causal_analysis",
		ShouldClarify:   false,
		TopicEntities:   []string{"物流", "供給網"},
	}, nil)

	// Retrieve should be called with the RESOLVED query, not the original
	mockRetrieve.On("Execute", mock.Anything, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "世界的な物流混乱が発生した背景と直接的原因"
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkText: "供給制約と港湾処理の逼迫が輸送停滞を招いた",
				Title:     "Global logistics disruption analysis",
				URL:       "https://example.com/logistics",
				Score:     0.85,
				ChunkID:   uuid.New(),
			},
		},
		ExpandedQueries: []string{"物流混乱 原因", "global supply disruption causes"},
	}, nil)
	mockRetrieve.On("Execute", mock.Anything, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkText: "世界的な物流混乱は供給制約と港湾滞留の組み合わせで悪化した",
				Title:     "Detailed logistics disruption analysis",
				URL:       "https://example.com/logistics-detail",
				Score:     0.92,
				ChunkID:   uuid.New(),
			},
		},
		ExpandedQueries: []string{"物流混乱 原因", "global supply disruption causes"},
	}, nil)

	// LLM Chat returns a valid JSON answer
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(&domain.LLMResponse{
		Text: `{"answer":"短い","citations":[{"chunk_id":"1","reason":"短い"}],"fallback":false,"reason":""}`,
	}, nil).Once()
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(&domain.LLMResponse{
		Text: `{"answer":"世界的な物流混乱は、供給制約、港湾滞留、輸送網の同期失敗が重なって発生しました。[1]","citations":[{"chunk_id":"1","reason":"供給制約による影響"}],"fallback":false,"reason":""}`,
	}, nil)

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve,
		usecase.NewXMLPromptBuilder(),
		mockLLM,
		usecase.NewOutputValidator(20),
		7, 512, 6000,
		"v1", "ja",
		testLogger,
		usecase.WithQueryPlanner(mockQP),
	)

	output, err := uc.Execute(context.Background(), usecase.AnswerWithRAGInput{
		Query: "世界的な物流混乱はなぜ起きた？",
	})

	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.False(t, output.Fallback)
	assert.Contains(t, output.Answer, "供給制約")

	// Verify QueryPlanner was called
	mockQP.AssertCalled(t, "PlanQuery", mock.Anything, mock.Anything)

	// Verify retrieval used resolved query
	mockRetrieve.AssertCalled(t, "Execute", mock.Anything, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "世界的な物流混乱が発生した背景と直接的原因"
	}))
}

func TestExecute_WithQueryPlanner_PropagatesPlannedIntentToStrategy(t *testing.T) {
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	mockQP := new(mockQueryPlannerPort)
	mockStrategy := &mockRetrievalStrategy{name: "causal-custom"}
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	chunkID := uuid.New()
	mockQP.On("PlanQuery", mock.Anything, mock.Anything).Return(&domain.QueryPlan{
		ResolvedQuery:   "世界的な物流混乱が発生した背景と直接的原因",
		SearchQueries:   []string{"物流混乱 原因", "global supply disruption causes"},
		Intent:          "causal_explanation",
		RetrievalPolicy: "global_only",
		AnswerFormat:    "causal_analysis",
	}, nil).Once()

	mockStrategy.On("Retrieve", mock.Anything,
		mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
			return input.Query == "世界的な物流混乱が発生した背景と直接的原因"
		}),
		mock.MatchedBy(func(intent usecase.QueryIntent) bool {
			return intent.IntentType == usecase.IntentCausalExplanation
		}),
	).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkID:   chunkID,
				ChunkText: "供給制約と港湾滞留が連鎖した",
				Title:     "Logistics disruption",
				URL:       "https://example.com/logistics-intent",
				Score:     0.91,
			},
		},
		ExpandedQueries: []string{"物流混乱 原因"},
	}, nil).Once()

	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Return(&domain.LLMResponse{
		Text: `{"answer":"供給制約と港湾滞留が重なり、物流混乱が連鎖しました。","citations":[{"chunk_id":"` + chunkID.String() + `","reason":"直接原因"}],"fallback":false,"reason":""}`,
		Done: true,
	}, nil).Once()

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve,
		usecase.NewXMLPromptBuilder(),
		mockLLM,
		usecase.NewOutputValidator(20),
		7, 512, 6000,
		"v1", "ja",
		testLogger,
		usecase.WithQueryPlanner(mockQP),
		usecase.WithStrategy(usecase.IntentCausalExplanation, mockStrategy),
	)

	output, err := uc.Execute(context.Background(), usecase.AnswerWithRAGInput{
		Query: "世界的な物流混乱はなぜ起きた？",
	})

	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.False(t, output.Fallback)
	assert.Contains(t, output.Answer, "供給制約")
	mockStrategy.AssertExpectations(t)
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

func TestExecute_WithQueryPlanner_NilRetrievalReturnsFallbackInsteadOfPanic(t *testing.T) {
	mockRetrieve := new(mockRetrieveContextUsecase)
	mockLLM := new(mockLLMClient)
	mockQP := new(mockQueryPlannerPort)
	mockStrategy := &mockRetrievalStrategy{name: "causal-custom"}
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	mockQP.On("PlanQuery", mock.Anything, mock.Anything).Return(&domain.QueryPlan{
		ResolvedQuery:   "物流危機の背景要因",
		SearchQueries:   []string{"物流危機 背景"},
		Intent:          "causal_explanation",
		RetrievalPolicy: "global_only",
		AnswerFormat:    "causal_analysis",
	}, nil).Once()

	mockStrategy.On("Retrieve", mock.Anything, mock.Anything, mock.Anything).
		Return((*usecase.RetrieveContextOutput)(nil), nil).Once()

	uc := usecase.NewAnswerWithRAGUsecase(
		mockRetrieve,
		usecase.NewXMLPromptBuilder(),
		mockLLM,
		usecase.NewOutputValidator(20),
		7, 512, 6000,
		"v1", "ja",
		testLogger,
		usecase.WithQueryPlanner(mockQP),
		usecase.WithStrategy(usecase.IntentCausalExplanation, mockStrategy),
	)

	output, err := uc.Execute(context.Background(), usecase.AnswerWithRAGInput{
		Query: "物流危機の背景を詳しく教えて",
	})

	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.True(t, output.Fallback)
	assert.Contains(t, output.Reason, "no context returned")
}
