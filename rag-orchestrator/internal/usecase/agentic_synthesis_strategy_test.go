package usecase

import (
	"context"
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockToolCaller struct {
	responses []*domain.LLMResponse
	calls     int
}

func (m *mockToolCaller) ChatWithTools(ctx context.Context, messages []domain.Message, tools []domain.ToolDefinition, maxTokens int) (*domain.LLMResponse, error) {
	if m.calls >= len(m.responses) {
		return &domain.LLMResponse{Text: "", Done: true}, nil
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

func TestAgenticSynthesisStrategy_UsesToolCalls(t *testing.T) {
	retrieve := &mockRetrieveUsecase{
		output: &RetrieveContextOutput{
			Contexts: []ContextItem{
				{ChunkID: uuid.New(), ChunkText: "Base context", Title: "Base", Score: 0.8},
			},
		},
	}
	tagTool := &mockTool{name: "tag_search", result: &domain.ToolResult{Data: "tag result", Success: true}}
	dispatcher := NewToolDispatcher(map[string]domain.Tool{
		"tag_search": tagTool,
	}, nil)
	planner := NewToolPlanner(&mockPlannerLLM{response: `{"steps":[{"tool_name":"tag_search","params":{"query":"test"}}]}`}, []domain.ToolDescriptor{
		{Name: "tag_search", Description: "Search tags"},
	}, nil)
	llm := &mockToolCaller{
		responses: []*domain.LLMResponse{
			{
				Text: "",
				ToolCalls: []domain.ToolCall{
					{Function: domain.ToolCallFunction{Name: "tag_search", Arguments: map[string]any{"query": "test"}}},
				},
				Done: true,
			},
			{
				Text: "tool loop complete",
				Done: true,
			},
		},
	}

	strategy := NewAgenticSynthesisStrategy(planner, dispatcher, retrieve, llm, nil)

	output, err := strategy.Retrieve(context.Background(), RetrieveContextInput{Query: "test"}, QueryIntent{
		IntentType:   IntentSynthesis,
		UserQuestion: "test",
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Contains(t, output.SupplementaryInfo, "tag result")
	assert.Contains(t, output.ToolsUsed, "tag_search")
	assert.GreaterOrEqual(t, llm.calls, 2)
}

func TestAgenticSynthesisStrategy_DeduplicatesRepeatedToolCalls(t *testing.T) {
	retrieve := &mockRetrieveUsecase{
		output: &RetrieveContextOutput{
			Contexts: []ContextItem{
				{ChunkID: uuid.New(), ChunkText: "Base context", Title: "Base", Score: 0.8},
			},
		},
	}
	tagTool := &mockTool{name: "tag_search", result: &domain.ToolResult{Data: "tag result", Success: true}}
	dispatcher := NewToolDispatcher(map[string]domain.Tool{
		"tag_search": tagTool,
	}, nil)
	planner := NewToolPlanner(&mockPlannerLLM{response: `{"steps":[{"tool_name":"tag_search","params":{"query":"test"}}]}`}, []domain.ToolDescriptor{
		{Name: "tag_search", Description: "Search tags"},
	}, nil)
	llm := &mockToolCaller{
		responses: []*domain.LLMResponse{
			{
				ToolCalls: []domain.ToolCall{
					{Function: domain.ToolCallFunction{Name: "tag_search", Arguments: map[string]any{"query": "test"}}},
				},
				Done: true,
			},
			{
				ToolCalls: []domain.ToolCall{
					{Function: domain.ToolCallFunction{Name: "tag_search", Arguments: map[string]any{"query": "test"}}},
				},
				Done: true,
			},
		},
	}

	strategy := NewAgenticSynthesisStrategy(planner, dispatcher, retrieve, llm, nil)
	output, err := strategy.Retrieve(context.Background(), RetrieveContextInput{Query: "test"}, QueryIntent{
		IntentType:   IntentSynthesis,
		UserQuestion: "test",
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Len(t, output.ToolsUsed, 1)
	assert.Len(t, output.SupplementaryInfo, 1)
}
