package usecase

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/domain"
)

type mockTool struct {
	name   string
	result *domain.ToolResult
	err    error
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return "mock tool" }
func (t *mockTool) Execute(_ context.Context, _ map[string]string) (*domain.ToolResult, error) {
	return t.result, t.err
}

func TestDispatch_TemporalIntent_SelectsDateRangeTool(t *testing.T) {
	dateTool := &mockTool{name: "date_range_filter", result: &domain.ToolResult{Data: "filtered", Success: true}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"date_range_filter": dateTool}, logger)

	intent := QueryIntent{IntentType: IntentTemporal, UserQuestion: "最近のAIニュース"}
	results := dispatcher.Dispatch(context.Background(), intent, "最近のAIニュース")

	if len(results) == 0 {
		t.Fatal("expected at least 1 tool result for temporal intent")
	}
	if !results[0].Success {
		t.Error("expected tool result to be successful")
	}
}

func TestDispatch_GeneralIntent_NoToolsSelected(t *testing.T) {
	dateTool := &mockTool{name: "date_range_filter", result: &domain.ToolResult{Data: "filtered", Success: true}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"date_range_filter": dateTool}, logger)

	intent := QueryIntent{IntentType: IntentGeneral, UserQuestion: "AI技術のトレンド"}
	results := dispatcher.Dispatch(context.Background(), intent, "AI技術のトレンド")

	if len(results) != 0 {
		t.Errorf("expected 0 tool results for general intent, got %d", len(results))
	}
}

func TestDispatch_ToolError_LogsAndContinues(t *testing.T) {
	failTool := &mockTool{name: "date_range_filter", result: nil, err: context.DeadlineExceeded}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"date_range_filter": failTool}, logger)

	intent := QueryIntent{IntentType: IntentTemporal, UserQuestion: "最近のニュース"}
	results := dispatcher.Dispatch(context.Background(), intent, "最近のニュース")

	// Should not panic, returns empty results on error
	if len(results) != 0 {
		t.Errorf("expected 0 results when tool errors, got %d", len(results))
	}
}

func TestDispatch_ComparisonIntent_SelectsTagSearchTool(t *testing.T) {
	tagTool := &mockTool{name: "tag_search", result: &domain.ToolResult{Data: "tags", Success: true}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"tag_search": tagTool}, logger)

	intent := QueryIntent{IntentType: IntentComparison, UserQuestion: "RustとGoの違い"}
	results := dispatcher.Dispatch(context.Background(), intent, "RustとGoの違い")

	if len(results) == 0 {
		t.Fatal("expected at least 1 tool result for comparison intent")
	}
}

func TestSelectTools_ArticleScopedRelatedArticles_ReturnsRelatedArticlesTool(t *testing.T) {
	relatedTool := &mockTool{name: "related_articles", result: &domain.ToolResult{Data: "Related articles:\n- Article A\n", Success: true}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"related_articles": relatedTool}, logger)

	intent := QueryIntent{
		IntentType:    IntentArticleScoped,
		SubIntentType: SubIntentRelatedArticles,
		UserQuestion:  "関連する記事はある？",
	}
	results := dispatcher.Dispatch(context.Background(), intent, "関連する記事はある？")

	if len(results) == 0 {
		t.Fatal("expected related_articles tool result for article-scoped + related_articles sub-intent")
	}
	if !results[0].Success {
		t.Error("expected tool result to be successful")
	}
}

func TestDispatchForPlan_ToolOnly_RelatedArticles(t *testing.T) {
	relatedTool := &mockTool{name: "related_articles", result: &domain.ToolResult{Data: "Related articles:\n- Article B\n", Success: true}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"related_articles": relatedTool}, logger)

	plan := &domain.PlannerOutput{
		Operation:       domain.OpRelatedArticles,
		RetrievalPolicy: domain.PolicyToolOnly,
	}
	intent := QueryIntent{
		IntentType:   IntentArticleScoped,
		ArticleID:    "article-123",
		UserQuestion: "関連する記事はある？",
	}
	results := dispatcher.DispatchForPlan(context.Background(), plan, intent, "関連する記事はある？")

	if len(results) == 0 {
		t.Fatal("expected related_articles tool result for tool_only policy")
	}
	if results[0].ToolName != "related_articles" {
		t.Errorf("expected tool name 'related_articles', got %q", results[0].ToolName)
	}
}

func TestDispatchForPlan_NonToolPolicy_FallsBackToIntent(t *testing.T) {
	dateTool := &mockTool{name: "date_range_filter", result: &domain.ToolResult{Data: "filtered", Success: true}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"date_range_filter": dateTool}, logger)

	plan := &domain.PlannerOutput{
		Operation:       domain.OpGeneral,
		RetrievalPolicy: domain.PolicyGlobalOnly,
	}
	intent := QueryIntent{IntentType: IntentTemporal, UserQuestion: "最近のニュース"}
	results := dispatcher.DispatchForPlan(context.Background(), plan, intent, "最近のニュース")

	if len(results) == 0 {
		t.Fatal("expected date_range_filter tool result from intent-based fallback")
	}
}

// --- ExecutePlan tests ---

func TestExecutePlan_ParallelExecution(t *testing.T) {
	toolA := &mockTool{name: "tool_a", result: &domain.ToolResult{Data: "result_a", Success: true}}
	toolB := &mockTool{name: "tool_b", result: &domain.ToolResult{Data: "result_b", Success: true}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"tool_a": toolA, "tool_b": toolB}, logger)

	plan := &domain.ToolPlan{
		Steps: []domain.ToolStep{
			{ToolName: "tool_a", Params: map[string]string{"query": "test"}},
			{ToolName: "tool_b", Params: map[string]string{"query": "test"}},
		},
	}

	results := dispatcher.ExecutePlan(context.Background(), plan)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Both should succeed
	for i, r := range results {
		if !r.Success {
			t.Errorf("step %d should succeed", i)
		}
	}
}

func TestExecutePlan_Dependencies_Sequential(t *testing.T) {
	toolA := &mockTool{name: "tool_a", result: &domain.ToolResult{Data: "tag_result", Success: true}}
	toolB := &mockTool{name: "tool_b", result: &domain.ToolResult{Data: "result_b", Success: true}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"tool_a": toolA, "tool_b": toolB}, logger)

	plan := &domain.ToolPlan{
		Steps: []domain.ToolStep{
			{ToolName: "tool_a", Params: map[string]string{"query": "test"}},
			{ToolName: "tool_b", Params: map[string]string{"query": "test"}, DependsOn: []int{0}},
		},
	}

	results := dispatcher.ExecutePlan(context.Background(), plan)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Step 1 (tool_b) should have run after step 0 completed
	if !results[0].Success || !results[1].Success {
		t.Error("both steps should succeed")
	}
}

func TestExecutePlan_ToolFailure_ContinuesOthers(t *testing.T) {
	failTool := &mockTool{name: "fail_tool", result: nil, err: context.DeadlineExceeded}
	goodTool := &mockTool{name: "good_tool", result: &domain.ToolResult{Data: "ok", Success: true}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"fail_tool": failTool, "good_tool": goodTool}, logger)

	plan := &domain.ToolPlan{
		Steps: []domain.ToolStep{
			{ToolName: "fail_tool", Params: map[string]string{"query": "test"}},
			{ToolName: "good_tool", Params: map[string]string{"query": "test"}},
		},
	}

	results := dispatcher.ExecutePlan(context.Background(), plan)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Success {
		t.Error("failed tool should have Success=false")
	}
	if !results[1].Success {
		t.Error("good tool should succeed despite other tool failure")
	}
}

func TestExecutePlan_UnknownTool_Skipped(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{}, logger)

	plan := &domain.ToolPlan{
		Steps: []domain.ToolStep{
			{ToolName: "nonexistent", Params: map[string]string{"query": "test"}},
		},
	}

	results := dispatcher.ExecutePlan(context.Background(), plan)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("unknown tool should not succeed")
	}
}

func TestSelectTools_ArticleScopedDetail_ReturnsNil(t *testing.T) {
	relatedTool := &mockTool{name: "related_articles", result: &domain.ToolResult{Data: "data", Success: true}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dispatcher := NewToolDispatcher(map[string]domain.Tool{"related_articles": relatedTool}, logger)

	intent := QueryIntent{
		IntentType:    IntentArticleScoped,
		SubIntentType: SubIntentDetail,
		UserQuestion:  "技術的な詳細をもっと教えて",
	}
	results := dispatcher.Dispatch(context.Background(), intent, "技術的な詳細をもっと教えて")

	if len(results) != 0 {
		t.Errorf("expected 0 tool results for article-scoped + detail sub-intent, got %d", len(results))
	}
}
