package tools

import (
	"context"
	"fmt"
	"strings"

	"rag-orchestrator/internal/domain"
)

// SummarizeForContextTool summarizes an article on-demand via pre-processor.
// Useful for condensing long articles to fit within the LLM context window.
type SummarizeForContextTool struct {
	client domain.SummarizerClient
}

// NewSummarizeForContextTool creates a new summarize-for-context tool.
func NewSummarizeForContextTool(client domain.SummarizerClient) *SummarizeForContextTool {
	return &SummarizeForContextTool{client: client}
}

func (t *SummarizeForContextTool) Name() string { return "summarize_for_context" }
func (t *SummarizeForContextTool) Description() string {
	return "Summarize a long article for context. Params: article_id"
}

func (t *SummarizeForContextTool) Execute(ctx context.Context, params map[string]string) (*domain.ToolResult, error) {
	articleID := strings.TrimSpace(params["article_id"])
	if articleID == "" {
		return &domain.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Error:    "article_id is required",
		}, nil
	}

	summary, err := t.client.Summarize(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("summarize_for_context failed: %w", err)
	}

	if summary == "" {
		return &domain.ToolResult{
			ToolName: t.Name(),
			Data:     "no summary available for article: " + articleID,
			Success:  true,
		}, nil
	}

	return &domain.ToolResult{
		ToolName: t.Name(),
		Data:     summary,
		Success:  true,
	}, nil
}
