package tools

import (
	"context"
	"testing"
)

type mockSummarizerClient struct {
	summary string
	err     error
}

func (m *mockSummarizerClient) Summarize(ctx context.Context, articleID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.summary, nil
}

func TestSummarizeForContextTool_Name(t *testing.T) {
	tool := NewSummarizeForContextTool(nil)
	if tool.Name() != "summarize_for_context" {
		t.Errorf("expected name summarize_for_context, got %s", tool.Name())
	}
}

func TestSummarizeForContextTool_ReturnsSummary(t *testing.T) {
	client := &mockSummarizerClient{
		summary: "ニューヨーク近代美術館は世界有数の美術館であり、20世紀の前衛芸術の中心地として発展した。",
	}
	tool := NewSummarizeForContextTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{
		"article_id": "123e4567-e89b-12d3-a456-426614174000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !containsString(result.Data, "ニューヨーク近代美術館") {
		t.Error("result should contain summary text")
	}
}

func TestSummarizeForContextTool_EmptyArticleID(t *testing.T) {
	tool := NewSummarizeForContextTool(nil)
	result, err := tool.Execute(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for empty article_id")
	}
}

func TestSummarizeForContextTool_EmptySummary(t *testing.T) {
	client := &mockSummarizerClient{summary: ""}
	tool := NewSummarizeForContextTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{
		"article_id": "some-id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success even with empty summary")
	}
	if !containsString(result.Data, "no summary") {
		t.Error("expected 'no summary' message")
	}
}

func TestSummarizeForContextTool_ClientError(t *testing.T) {
	client := &mockSummarizerClient{err: context.DeadlineExceeded}
	tool := NewSummarizeForContextTool(client)

	_, err := tool.Execute(context.Background(), map[string]string{
		"article_id": "some-id",
	})
	if err == nil {
		t.Error("expected error when client fails")
	}
}
