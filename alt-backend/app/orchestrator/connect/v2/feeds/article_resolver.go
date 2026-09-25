package feeds

import (
	"context"
)

// resolveArticle resolves article ID and content using DB cache, request fallback, or URL scraping.
func (h *Handler) resolveArticle(ctx context.Context, feedURL, articleID, content, title string) (string, string, string, error) {
	return h.deps.ResolveArticle.ResolveArticle(ctx, feedURL, articleID, content, title)
}
