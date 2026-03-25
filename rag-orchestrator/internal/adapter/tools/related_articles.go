package tools

import (
	"context"
	"fmt"
	"strings"

	"rag-orchestrator/internal/domain"
)

// RelatedArticlesTool finds articles related to a given query using search.
type RelatedArticlesTool struct {
	client domain.SearchClient
}

// NewRelatedArticlesTool creates a new related articles tool.
func NewRelatedArticlesTool(client domain.SearchClient) *RelatedArticlesTool {
	return &RelatedArticlesTool{client: client}
}

func (t *RelatedArticlesTool) Name() string        { return "related_articles" }
func (t *RelatedArticlesTool) Description() string { return "Find articles related to a topic" }

func (t *RelatedArticlesTool) Execute(ctx context.Context, params map[string]string) (*domain.ToolResult, error) {
	query := params["query"]
	if query == "" {
		return &domain.ToolResult{Success: false, Error: "query is required"}, nil
	}

	hits, err := t.client.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("related articles search failed: %w", err)
	}

	if len(hits) == 0 {
		return &domain.ToolResult{Data: "no related articles found", Success: true}, nil
	}

	var sb strings.Builder
	sb.WriteString("Related articles:\n")
	for i, hit := range hits {
		if i >= 5 {
			break
		}
		fmt.Fprintf(&sb, "- %s\n", hit.Title)
	}

	return &domain.ToolResult{Data: sb.String(), Success: true}, nil
}
