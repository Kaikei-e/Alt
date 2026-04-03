package tools

import (
	"context"
	"testing"

	"rag-orchestrator/internal/domain"
)

type mockTagExtractorClient struct {
	tags []domain.ExtractedTag
	err  error
}

func (m *mockTagExtractorClient) ExtractTags(ctx context.Context, text string) ([]domain.ExtractedTag, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.tags, nil
}

func TestExtractQueryTagsTool_Name(t *testing.T) {
	tool := NewExtractQueryTagsTool(nil)
	if tool.Name() != "extract_query_tags" {
		t.Errorf("expected name extract_query_tags, got %s", tool.Name())
	}
}

func TestExtractQueryTagsTool_ReturnsTags(t *testing.T) {
	client := &mockTagExtractorClient{
		tags: []domain.ExtractedTag{
			{Name: "New York", Confidence: 0.95},
			{Name: "art", Confidence: 0.88},
			{Name: "culture", Confidence: 0.72},
		},
	}
	tool := NewExtractQueryTagsTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{
		"query": "ニューヨークと芸術のかかわり",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !containsString(result.Data, "New York") {
		t.Error("result should contain extracted tag 'New York'")
	}
	if !containsString(result.Data, "0.95") {
		t.Error("result should contain confidence score")
	}
}

func TestExtractQueryTagsTool_EmptyQuery(t *testing.T) {
	tool := NewExtractQueryTagsTool(nil)
	result, err := tool.Execute(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for empty query")
	}
}

func TestExtractQueryTagsTool_NoTags(t *testing.T) {
	client := &mockTagExtractorClient{tags: nil}
	tool := NewExtractQueryTagsTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{
		"query": "something obscure",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success even with no tags")
	}
	if !containsString(result.Data, "no tags") {
		t.Error("expected 'no tags' message")
	}
}

func TestExtractQueryTagsTool_ClientError(t *testing.T) {
	client := &mockTagExtractorClient{err: context.DeadlineExceeded}
	tool := NewExtractQueryTagsTool(client)

	_, err := tool.Execute(context.Background(), map[string]string{
		"query": "test",
	})
	if err == nil {
		t.Error("expected error when client fails")
	}
}
