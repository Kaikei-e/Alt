package tools

import (
	"context"
	"fmt"
	"strings"

	"rag-orchestrator/internal/domain"
)

// TagSearchTool searches articles by keyword/tag using the search indexer.
type TagSearchTool struct {
	client domain.SearchClient
}

// NewTagSearchTool creates a new tag search tool.
func NewTagSearchTool(client domain.SearchClient) *TagSearchTool {
	return &TagSearchTool{client: client}
}

func (t *TagSearchTool) Name() string        { return "tag_search" }
func (t *TagSearchTool) Description() string { return "Search articles by tag or keyword" }

func (t *TagSearchTool) Execute(ctx context.Context, params map[string]string) (*domain.ToolResult, error) {
	query := params["query"]
	if query == "" {
		return &domain.ToolResult{Success: false, Error: "query is required"}, nil
	}

	hits, err := t.client.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("tag search failed: %w", err)
	}

	if len(hits) == 0 {
		return &domain.ToolResult{Data: "no results", Success: true}, nil
	}

	var sb strings.Builder
	for i, hit := range hits {
		if i >= 5 {
			break
		}
		sb.WriteString(fmt.Sprintf("- %s: %s\n", hit.Title, truncateStr(hit.Content, 200)))
	}

	return &domain.ToolResult{Data: sb.String(), Success: true}, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
