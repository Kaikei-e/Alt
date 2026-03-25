package tools

import (
	"context"
	"fmt"
	"strings"

	"rag-orchestrator/internal/domain"
)

// DateRangeFilterTool filters articles by temporal keywords in the query.
type DateRangeFilterTool struct {
	client domain.SearchClient
}

// NewDateRangeFilterTool creates a new date range filter tool.
func NewDateRangeFilterTool(client domain.SearchClient) *DateRangeFilterTool {
	return &DateRangeFilterTool{client: client}
}

func (t *DateRangeFilterTool) Name() string        { return "date_range_filter" }
func (t *DateRangeFilterTool) Description() string { return "Filter articles by date range" }

func (t *DateRangeFilterTool) Execute(ctx context.Context, params map[string]string) (*domain.ToolResult, error) {
	query := params["query"]
	if query == "" {
		return &domain.ToolResult{Success: false, Error: "query is required"}, nil
	}

	// Use search client with the temporal query — the search engine
	// handles recency ranking internally via Meilisearch
	hits, err := t.client.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("date range filter failed: %w", err)
	}

	if len(hits) == 0 {
		return &domain.ToolResult{Data: "no recent results", Success: true}, nil
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
