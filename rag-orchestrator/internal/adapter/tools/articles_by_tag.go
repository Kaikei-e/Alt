package tools

import (
	"context"
	"fmt"
	"strings"

	"rag-orchestrator/internal/domain"
)

// ArticlesByTagTool finds articles by a specific tag using alt-backend FetchArticlesByTag.
type ArticlesByTagTool struct {
	client domain.ArticlesByTagClient
}

// NewArticlesByTagTool creates a new articles-by-tag tool.
func NewArticlesByTagTool(client domain.ArticlesByTagClient) *ArticlesByTagTool {
	return &ArticlesByTagTool{client: client}
}

func (t *ArticlesByTagTool) Name() string { return "articles_by_tag" }
func (t *ArticlesByTagTool) Description() string {
	return "Find articles by specific tag. Params: tag_name"
}

func (t *ArticlesByTagTool) Execute(ctx context.Context, params map[string]string) (*domain.ToolResult, error) {
	tagName := strings.TrimSpace(params["tag_name"])
	if tagName == "" {
		return &domain.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Error:    "tag_name is required",
		}, nil
	}

	articles, err := t.client.FetchArticlesByTag(ctx, tagName, 10)
	if err != nil {
		return nil, fmt.Errorf("articles_by_tag fetch failed: %w", err)
	}

	if len(articles) == 0 {
		return &domain.ToolResult{
			ToolName: t.Name(),
			Data:     "no articles found for tag: " + tagName,
			Success:  true,
		}, nil
	}

	// Limit to top 5
	if len(articles) > 5 {
		articles = articles[:5]
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Articles tagged '%s':\n", tagName)
	for _, a := range articles {
		content := a.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Fprintf(&sb, "- [%s](%s): %s\n", a.Title, a.URL, content)
	}

	return &domain.ToolResult{
		ToolName: t.Name(),
		Data:     sb.String(),
		Success:  true,
	}, nil
}
