package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ArticleMetadata represents minimal article info for temporal filtering
type ArticleMetadata struct {
	ID          uuid.UUID
	Title       string
	URL         string
	PublishedAt time.Time
	FeedID      uuid.UUID
	Tags        []string
}

// ArticleClient defines the interface for fetching article metadata from alt-backend
type ArticleClient interface {
	// GetRecentArticles returns articles published within the given duration
	GetRecentArticles(ctx context.Context, withinHours int, limit int) ([]ArticleMetadata, error)
}
