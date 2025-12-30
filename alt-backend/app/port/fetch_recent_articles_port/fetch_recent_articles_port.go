package fetch_recent_articles_port

import (
	"alt/domain"
	"context"
	"time"
)

// FetchRecentArticlesPort defines the interface for fetching recent articles
// Used by rag-orchestrator for temporal topics feature
type FetchRecentArticlesPort interface {
	// FetchRecentArticles retrieves articles published since the given time
	FetchRecentArticles(ctx context.Context, since time.Time, limit int) ([]*domain.Article, error)
}
