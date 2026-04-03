package usecase

import (
	"context"
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
)

type mockRetrieveUsecase struct {
	output *RetrieveContextOutput
	err    error
}

func (m *mockRetrieveUsecase) Execute(ctx context.Context, input RetrieveContextInput) (*RetrieveContextOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.output, nil
}

func TestSynthesisStrategy_Name(t *testing.T) {
	s := NewSynthesisStrategy(nil, nil, nil, nil)
	if s.Name() != "synthesis" {
		t.Errorf("expected name 'synthesis', got %s", s.Name())
	}
}

func TestSynthesisStrategy_UsesToolPlannerAndDispatcher(t *testing.T) {
	// Mock planner returns a plan with 2 tools
	llm := &mockPlannerLLM{
		response: `{"steps":[{"tool_name":"tag_cloud_explore","params":{"topic":"test"}},{"tool_name":"keyword_search","params":{"query":"test"}}]}`,
	}
	tools := []domain.ToolDescriptor{
		{Name: "tag_cloud_explore", Description: "Explore tags"},
		{Name: "keyword_search", Description: "Keyword search"},
	}
	planner := NewToolPlanner(llm, tools, nil)

	// Mock tools
	tagTool := &mockTool{name: "tag_cloud_explore", result: &domain.ToolResult{
		Data: "Related tags:\n- art (10 articles)\n- culture (5 articles)", Success: true,
	}}
	kwTool := &mockTool{name: "keyword_search", result: &domain.ToolResult{
		Data: "Search results for test", Success: true,
	}}
	toolMap := map[string]domain.Tool{
		"tag_cloud_explore": tagTool,
		"keyword_search":    kwTool,
	}
	dispatcher := NewToolDispatcher(toolMap, nil)

	// Mock retrieval for vector search fallback
	retrieve := &mockRetrieveUsecase{
		output: &RetrieveContextOutput{
			Contexts: []ContextItem{
				{ChunkID: uuid.New(), ChunkText: "Art in New York", Title: "NYC Art", Score: 0.8},
			},
		},
	}

	strategy := NewSynthesisStrategy(planner, dispatcher, retrieve, nil)

	intent := QueryIntent{IntentType: IntentSynthesis, UserQuestion: "ニューヨークと芸術のかかわり"}
	input := RetrieveContextInput{Query: "ニューヨークと芸術のかかわり"}

	output, err := strategy.Retrieve(context.Background(), input, intent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Contexts) == 0 {
		t.Error("expected at least 1 context from retrieval")
	}
	if len(output.ToolsUsed) == 0 {
		t.Error("expected tools to be recorded in ToolsUsed")
	}
	if len(output.SupplementaryInfo) == 0 {
		t.Error("expected supplementary info from tool results")
	}
}

func TestSynthesisStrategy_FallbackOnPlannerFailure(t *testing.T) {
	// LLM fails → fallback to general retrieval
	llm := &mockPlannerLLM{err: context.DeadlineExceeded}
	planner := NewToolPlanner(llm, nil, nil)
	dispatcher := NewToolDispatcher(map[string]domain.Tool{}, nil)

	retrieve := &mockRetrieveUsecase{
		output: &RetrieveContextOutput{
			Contexts: []ContextItem{
				{ChunkID: uuid.New(), ChunkText: "Fallback context", Title: "Fallback", Score: 0.6},
			},
		},
	}

	strategy := NewSynthesisStrategy(planner, dispatcher, retrieve, nil)
	intent := QueryIntent{IntentType: IntentSynthesis, UserQuestion: "test"}
	input := RetrieveContextInput{Query: "test"}

	output, err := strategy.Retrieve(context.Background(), input, intent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Contexts) == 0 {
		t.Error("expected fallback contexts")
	}
}
