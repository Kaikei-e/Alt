package tools_test

import (
	"context"
	"testing"

	"rag-orchestrator/internal/adapter/tools"
	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

// --- TagSearchTool ---

type mockSearchClient struct {
	hits []domain.SearchHit
	err  error
}

func (m *mockSearchClient) Search(_ context.Context, _ string) ([]domain.SearchHit, error) {
	return m.hits, m.err
}

func TestTagSearchTool_Name(t *testing.T) {
	tool := tools.NewTagSearchTool(&mockSearchClient{})
	assert.Equal(t, "tag_search", tool.Name())
}

func TestTagSearchTool_Execute_Success(t *testing.T) {
	client := &mockSearchClient{
		hits: []domain.SearchHit{
			{ID: "1", Title: "Article about Rust", Content: "Rust content"},
			{ID: "2", Title: "Article about Go", Content: "Go content"},
		},
	}
	tool := tools.NewTagSearchTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{"query": "Rust"})
	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Data, "Rust")
}

func TestTagSearchTool_Execute_Empty(t *testing.T) {
	client := &mockSearchClient{hits: nil}
	tool := tools.NewTagSearchTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{"query": "nonexistent"})
	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "no results", result.Data)
}

// --- DateRangeFilterTool ---

func TestDateRangeFilterTool_Name(t *testing.T) {
	tool := tools.NewDateRangeFilterTool(&mockSearchClient{})
	assert.Equal(t, "date_range_filter", tool.Name())
}

func TestDateRangeFilterTool_Execute(t *testing.T) {
	client := &mockSearchClient{
		hits: []domain.SearchHit{
			{ID: "1", Title: "Recent news", Content: "Today's article"},
		},
	}
	tool := tools.NewDateRangeFilterTool(client)

	result, err := tool.Execute(context.Background(), map[string]string{"query": "最近のAIニュース"})
	assert.NoError(t, err)
	assert.True(t, result.Success)
}
