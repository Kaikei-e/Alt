package tools

import (
	"context"
	"fmt"
	"strings"

	"rag-orchestrator/internal/domain"
)

// ArticleLookupTool fetches recent articles from alt-backend.
type ArticleLookupTool struct {
	client domain.ArticleClient
}

// NewArticleLookupTool creates a new article lookup tool.
func NewArticleLookupTool(client domain.ArticleClient) *ArticleLookupTool {
	return &ArticleLookupTool{client: client}
}

func (t *ArticleLookupTool) Name() string        { return "article_lookup" }
func (t *ArticleLookupTool) Description() string { return "Look up recent articles from the backend" }

func (t *ArticleLookupTool) Execute(ctx context.Context, params map[string]string) (*domain.ToolResult, error) {
	articles, err := t.client.GetRecentArticles(ctx, 168, 10) // last 7 days, top 10
	if err != nil {
		return nil, fmt.Errorf("article lookup failed: %w", err)
	}

	if len(articles) == 0 {
		return &domain.ToolResult{Data: "no recent articles found", Success: true}, nil
	}

	var sb strings.Builder
	for i, a := range articles {
		if i >= 5 {
			break
		}
		fmt.Fprintf(&sb, "- [%s] %s (%s)\n", a.ID, a.Title, a.PublishedAt.Format("2006-01-02"))
	}

	return &domain.ToolResult{Data: sb.String(), Success: true}, nil
}
