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
