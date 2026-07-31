package cached_article_tags_gateway

import (
	"alt/domain"
	"alt/orchestrator/port/cached_article_tags_port"
	"context"
)

// Verify interface compliance at compile time.
var _ cached_article_tags_port.CachedArticleTagsPort = (*Gateway)(nil)

// articleTagReader is the alt-data-hub read (capability catalog §2.J W3-J1).
type articleTagReader interface {
	FetchArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error)
}

// Gateway reads tags without triggering generation.
//
// It is the same procedure FetchArticleTagsGateway calls first; the difference
// is what happens on an empty answer. The SSE stream uses this one to decide
// whether it has something to send now, and must not start a generation as a
// side effect of asking.
type Gateway struct {
	tags articleTagReader
}

func NewGateway(tags articleTagReader) *Gateway {
	if tags == nil {
		panic("cached_article_tags_gateway: a tag reader is required (see .claude/rules/di-wiring.md)")
	}
	return &Gateway{tags: tags}
}

// FetchCachedArticleTags retrieves tags without triggering on-the-fly generation.
func (g *Gateway) FetchCachedArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error) {
	return g.tags.FetchArticleTags(ctx, articleID)
}
