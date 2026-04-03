package tools

import (
	"context"
	"testing"

	"rag-orchestrator/internal/domain"
)

// mockArticleServiceClient implements the subset of ArticleServiceClient needed for TagCloudExploreTool.
type mockTagCloudClient struct {
	tags []domain.TagCloudEntry
	err  error
}

func (m *mockTagCloudClient) FetchTagCloud(ctx context.Context, limit int) ([]domain.TagCloudEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.tags, nil
}

func TestTagCloudExploreTool_Name(t *testing.T) {
	tool := NewTagCloudExploreTool(nil)
	if tool.Name() != "tag_cloud_explore" {
		t.Errorf("expected name tag_cloud_explore, got %s", tool.Name())
	}
}

func TestTagCloudExploreTool_FiltersByTopic(t *testing.T) {
	client := &mockTagCloudClient{
		tags: []domain.TagCloudEntry{
			{TagName: "New York", ArticleCount: 25},
			{TagName: "art exhibition", ArticleCount: 12},
			{TagName: "art market", ArticleCount: 8},
			{TagName: "Bitcoin", ArticleCount: 50},
			{TagName: "climate change", ArticleCount: 40},
			{TagName: "New York art", ArticleCount: 5},
			{TagName: "芸術", ArticleCount: 3},
		},
	}
	tool := NewTagCloudExploreTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{
		"topic": "ニューヨーク 芸術 art",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	// Should contain art-related tags, not Bitcoin or climate
	if containsString(result.Data, "Bitcoin") {
		t.Error("result should not contain unrelated tag 'Bitcoin'")
	}
	if !containsString(result.Data, "art") {
		t.Error("result should contain art-related tags")
	}
	if !containsString(result.Data, "芸術") {
		t.Error("result should contain '芸術' (matches keyword)")
	}
}

func TestTagCloudExploreTool_EmptyTopic(t *testing.T) {
	tool := NewTagCloudExploreTool(nil)
	result, err := tool.Execute(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for empty topic")
	}
}

func TestTagCloudExploreTool_NoMatchingTags(t *testing.T) {
	client := &mockTagCloudClient{
		tags: []domain.TagCloudEntry{
			{TagName: "Bitcoin", ArticleCount: 50},
			{TagName: "climate change", ArticleCount: 40},
		},
	}
	tool := NewTagCloudExploreTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{
		"topic": "ニューヨーク 芸術",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success even with no matches")
	}
	if !containsString(result.Data, "no matching tags") {
		t.Error("expected 'no matching tags' message")
	}
}

func TestTagCloudExploreTool_ClientError(t *testing.T) {
	client := &mockTagCloudClient{
		err: context.DeadlineExceeded,
	}
	tool := NewTagCloudExploreTool(client)

	_, err := tool.Execute(context.Background(), map[string]string{
		"topic": "art",
	})
	if err == nil {
		t.Error("expected error when client fails")
	}
}

func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && contains(s, substr)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
