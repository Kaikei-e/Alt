package tools

import (
	"context"
	"fmt"
	"strings"

	"rag-orchestrator/internal/domain"
)

// SearchRecapsTool searches summarized topic clusters by tag.
// Uses search-indexer/recap-worker SearchRecapsByTag RPC.
type SearchRecapsTool struct {
	client domain.RecapSearchClient
}

// NewSearchRecapsTool creates a new search recaps tool.
func NewSearchRecapsTool(client domain.RecapSearchClient) *SearchRecapsTool {
	return &SearchRecapsTool{client: client}
}

func (t *SearchRecapsTool) Name() string { return "search_recaps" }
func (t *SearchRecapsTool) Description() string {
	return "Find summarized topic clusters by tag. Params: tag_name"
}

func (t *SearchRecapsTool) Execute(ctx context.Context, params map[string]string) (*domain.ToolResult, error) {
	tagName := strings.TrimSpace(params["tag_name"])
	if tagName == "" {
		return &domain.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Error:    "tag_name is required",
		}, nil
	}

	results, err := t.client.SearchRecapsByTag(ctx, tagName, 5)
	if err != nil {
		return nil, fmt.Errorf("search_recaps failed: %w", err)
	}

	if len(results) == 0 {
		return &domain.ToolResult{
			ToolName: t.Name(),
			Data:     "no recaps found for tag: " + tagName,
			Success:  true,
		}, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Recap summaries for '%s':\n", tagName)
	for _, r := range results {
		fmt.Fprintf(&sb, "### %s\n%s\nKey terms: %s\n\n",
			r.Genre, r.Summary, strings.Join(r.TopTerms, ", "))
	}

	return &domain.ToolResult{
		ToolName: t.Name(),
		Data:     sb.String(),
		Success:  true,
	}, nil
}
