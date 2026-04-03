package tools

import (
	"context"
	"fmt"
	"strings"

	"rag-orchestrator/internal/domain"
)

// ExtractQueryTagsTool extracts semantic tags from a query using tag-generator via mq-hub.
type ExtractQueryTagsTool struct {
	client domain.TagExtractorClient
}

// NewExtractQueryTagsTool creates a new extract query tags tool.
func NewExtractQueryTagsTool(client domain.TagExtractorClient) *ExtractQueryTagsTool {
	return &ExtractQueryTagsTool{client: client}
}

func (t *ExtractQueryTagsTool) Name() string { return "extract_query_tags" }
func (t *ExtractQueryTagsTool) Description() string {
	return "Extract semantic tags from text. Params: query"
}

func (t *ExtractQueryTagsTool) Execute(ctx context.Context, params map[string]string) (*domain.ToolResult, error) {
	query := strings.TrimSpace(params["query"])
	if query == "" {
		return &domain.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Error:    "query is required",
		}, nil
	}

	tags, err := t.client.ExtractTags(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("extract_query_tags failed: %w", err)
	}

	if len(tags) == 0 {
		return &domain.ToolResult{
			ToolName: t.Name(),
			Data:     "no tags extracted from query: " + query,
			Success:  true,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("Extracted tags:\n")
	for _, tag := range tags {
		fmt.Fprintf(&sb, "- %s (confidence: %.2f)\n", tag.Name, tag.Confidence)
	}

	return &domain.ToolResult{
		ToolName: t.Name(),
		Data:     sb.String(),
		Success:  true,
	}, nil
}
