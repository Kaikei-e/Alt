package usecase

import (
	"context"
	"errors"
	"testing"

	"rag-orchestrator/internal/domain"
)

type mockPlannerLLM struct {
	response string
	err      error
}

func (m *mockPlannerLLM) Generate(ctx context.Context, prompt string, maxTokens int) (*domain.LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &domain.LLMResponse{Text: m.response}, nil
}

func (m *mockPlannerLLM) Chat(ctx context.Context, messages []domain.Message, maxTokens int) (*domain.LLMResponse, error) {
	return m.Generate(ctx, "", maxTokens)
}

func (m *mockPlannerLLM) GenerateStream(ctx context.Context, prompt string, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	return nil, nil, errors.New("not implemented")
}

func (m *mockPlannerLLM) ChatStream(ctx context.Context, messages []domain.Message, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	return nil, nil, errors.New("not implemented")
}

func (m *mockPlannerLLM) Version() string { return "mock" }

func TestToolPlanner_GeneratesValidPlan(t *testing.T) {
	llm := &mockPlannerLLM{
		response: `{"steps":[{"tool_name":"tag_cloud_explore","params":{"topic":"ニューヨーク 芸術"}},{"tool_name":"vector_search","params":{"query":"NYC art museums"}},{"tool_name":"articles_by_tag","params":{"tag_name":"art"},"depends_on":[0]}]}`,
	}
	tools := []domain.ToolDescriptor{
		{Name: "tag_cloud_explore", Description: "Explore tags"},
		{Name: "vector_search", Description: "Vector search"},
		{Name: "articles_by_tag", Description: "Articles by tag"},
	}
	planner := NewToolPlanner(llm, tools, nil)

	plan, err := planner.Plan(context.Background(), "ニューヨークと芸術のかかわり")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(plan.Steps))
	}
	if plan.Steps[0].ToolName != "tag_cloud_explore" {
		t.Errorf("expected first step tag_cloud_explore, got %s", plan.Steps[0].ToolName)
	}
	if plan.Steps[2].ToolName != "articles_by_tag" {
		t.Errorf("expected third step articles_by_tag, got %s", plan.Steps[2].ToolName)
	}
	if len(plan.Steps[2].DependsOn) != 1 || plan.Steps[2].DependsOn[0] != 0 {
		t.Errorf("expected step 2 to depend on step 0, got %v", plan.Steps[2].DependsOn)
	}
}

func TestToolPlanner_FiltersUnknownTools(t *testing.T) {
	llm := &mockPlannerLLM{
		response: `{"steps":[{"tool_name":"tag_cloud_explore","params":{"topic":"test"}},{"tool_name":"unknown_tool","params":{}}]}`,
	}
	tools := []domain.ToolDescriptor{
		{Name: "tag_cloud_explore", Description: "Explore tags"},
	}
	planner := NewToolPlanner(llm, tools, nil)

	plan, err := planner.Plan(context.Background(), "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// unknown_tool should be filtered out
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step after filtering, got %d", len(plan.Steps))
	}
	if plan.Steps[0].ToolName != "tag_cloud_explore" {
		t.Errorf("expected tag_cloud_explore, got %s", plan.Steps[0].ToolName)
	}
}

func TestToolPlanner_InvalidJSON_FallsBackToDefault(t *testing.T) {
	llm := &mockPlannerLLM{
		response: "This is not valid JSON at all",
	}
	tools := []domain.ToolDescriptor{
		{Name: "tag_cloud_explore", Description: "Explore tags"},
		{Name: "keyword_search", Description: "Keyword search"},
		{Name: "vector_search", Description: "Vector search"},
	}
	planner := NewToolPlanner(llm, tools, nil)

	plan, err := planner.Plan(context.Background(), "ニューヨークと芸術")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return default plan, not empty
	if len(plan.Steps) == 0 {
		t.Fatal("expected default plan with steps, got empty")
	}
}

func TestToolPlanner_LLMError_FallsBackToDefault(t *testing.T) {
	llm := &mockPlannerLLM{
		err: context.DeadlineExceeded,
	}
	tools := []domain.ToolDescriptor{
		{Name: "tag_cloud_explore", Description: "Explore tags"},
		{Name: "keyword_search", Description: "Keyword search"},
	}
	planner := NewToolPlanner(llm, tools, nil)

	plan, err := planner.Plan(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("expected default plan on LLM error")
	}
}

func TestToolPlanner_EmptySteps_FallsBackToDefault(t *testing.T) {
	llm := &mockPlannerLLM{
		response: `{"steps":[]}`,
	}
	tools := []domain.ToolDescriptor{
		{Name: "tag_cloud_explore", Description: "Explore tags"},
	}
	planner := NewToolPlanner(llm, tools, nil)

	plan, err := planner.Plan(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("expected default plan when LLM returns empty steps")
	}
}
