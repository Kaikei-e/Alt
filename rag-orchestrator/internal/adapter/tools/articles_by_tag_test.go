package tools

import (
	"context"
	"testing"

	"rag-orchestrator/internal/domain"
)

type mockArticlesByTagClient struct {
	articles []domain.TagArticle
	err      error
}

func (m *mockArticlesByTagClient) FetchArticlesByTag(ctx context.Context, tagName string, limit int) ([]domain.TagArticle, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.articles, nil
}

func TestArticlesByTagTool_Name(t *testing.T) {
	tool := NewArticlesByTagTool(nil)
	if tool.Name() != "articles_by_tag" {
		t.Errorf("expected name articles_by_tag, got %s", tool.Name())
	}
}

func TestArticlesByTagTool_ReturnsArticles(t *testing.T) {
	client := &mockArticlesByTagClient{
		articles: []domain.TagArticle{
			{ID: "1", Title: "MoMA展覧会レポート", URL: "https://example.com/1", Content: "ニューヨーク近代美術館で開催中の..."},
			{ID: "2", Title: "NYC Art Scene 2026", URL: "https://example.com/2", Content: "The vibrant art scene in New York..."},
		},
	}
	tool := NewArticlesByTagTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{
		"tag_name": "art",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !containsString(result.Data, "MoMA") {
		t.Error("result should contain article title")
	}
	if !containsString(result.Data, "https://example.com/1") {
		t.Error("result should contain article URL")
	}
}

func TestArticlesByTagTool_EmptyTagName(t *testing.T) {
	tool := NewArticlesByTagTool(nil)
	result, err := tool.Execute(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for empty tag_name")
	}
}

func TestArticlesByTagTool_NoResults(t *testing.T) {
	client := &mockArticlesByTagClient{articles: nil}
	tool := NewArticlesByTagTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{
		"tag_name": "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success with no results")
	}
	if !containsString(result.Data, "no articles") {
		t.Error("expected 'no articles' message")
	}
}

func TestArticlesByTagTool_ClientError(t *testing.T) {
	client := &mockArticlesByTagClient{err: context.DeadlineExceeded}
	tool := NewArticlesByTagTool(client)

	_, err := tool.Execute(context.Background(), map[string]string{
		"tag_name": "art",
	})
	if err == nil {
		t.Error("expected error when client fails")
	}
}
