package tools

import (
	"context"
	"testing"

	"rag-orchestrator/internal/domain"
)

type mockRecapSearchClient struct {
	results []domain.RecapSearchResult
	err     error
}

func (m *mockRecapSearchClient) SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]domain.RecapSearchResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func TestSearchRecapsTool_Name(t *testing.T) {
	tool := NewSearchRecapsTool(nil)
	if tool.Name() != "search_recaps" {
		t.Errorf("expected name search_recaps, got %s", tool.Name())
	}
}

func TestSearchRecapsTool_ReturnsRecaps(t *testing.T) {
	client := &mockRecapSearchClient{
		results: []domain.RecapSearchResult{
			{Genre: "Art & Culture", Summary: "NYC art scene expanding with new galleries", TopTerms: []string{"art", "gallery", "NYC"}},
			{Genre: "Entertainment", Summary: "Broadway shows break attendance records", TopTerms: []string{"Broadway", "theater"}},
		},
	}
	tool := NewSearchRecapsTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{
		"tag_name": "art",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !containsString(result.Data, "Art & Culture") {
		t.Error("result should contain genre name")
	}
	if !containsString(result.Data, "NYC art scene") {
		t.Error("result should contain summary text")
	}
}

func TestSearchRecapsTool_EmptyTagName(t *testing.T) {
	tool := NewSearchRecapsTool(nil)
	result, err := tool.Execute(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for empty tag_name")
	}
}

func TestSearchRecapsTool_NoResults(t *testing.T) {
	client := &mockRecapSearchClient{results: nil}
	tool := NewSearchRecapsTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{
		"tag_name": "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success with no results")
	}
	if !containsString(result.Data, "no recaps") {
		t.Error("expected 'no recaps' message")
	}
}

func TestSearchRecapsTool_ClientError(t *testing.T) {
	client := &mockRecapSearchClient{err: context.DeadlineExceeded}
	tool := NewSearchRecapsTool(client)

	_, err := tool.Execute(context.Background(), map[string]string{
		"tag_name": "art",
	})
	if err == nil {
		t.Error("expected error when client fails")
	}
}
