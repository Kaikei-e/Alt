package knowledge_home_port

import (
	"alt/domain"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrDismissTargetNotFound = errors.New("knowledge home dismiss target not found")

// GetKnowledgeHomeItemsPort reads items from the knowledge home projection.
type GetKnowledgeHomeItemsPort interface {
	GetKnowledgeHomeItems(ctx context.Context, userID uuid.UUID, cursor string, limit int, filter *domain.KnowledgeHomeLensFilter) ([]domain.KnowledgeHomeItem, string, bool, error)
}

// UpsertKnowledgeHomeItemPort writes items to the knowledge home projection.
type UpsertKnowledgeHomeItemPort interface {
	UpsertKnowledgeHomeItem(ctx context.Context, item domain.KnowledgeHomeItem) error
}

// DismissKnowledgeHomeItemPort marks an item as dismissed so it no longer appears in Home.
type DismissKnowledgeHomeItemPort interface {
	DismissKnowledgeHomeItem(ctx context.Context, userID uuid.UUID, itemKey string, projectionVersion int, dismissedAt time.Time) error
}

// ClearSupersedeStatePort clears supersede state for an item after user acknowledgement (e.g. open).
type ClearSupersedeStatePort interface {
	ClearSupersedeState(ctx context.Context, userID uuid.UUID, itemKey string, projectionVersion int) error
}

// ListDistinctUserIDsPort returns distinct user IDs that have knowledge home items.
// Used by scheduled jobs (RecallProjector, DigestReconcile) to discover active users
// instead of relying on static AllowedUserIDs from configuration.
type ListDistinctUserIDsPort interface {
	ListDistinctUserIDs(ctx context.Context) ([]uuid.UUID, error)
}

// TagArticleCount holds a tag name and its article count for a given time period.
type TagArticleCount struct {
	TagName      string
	ArticleCount int
}

// FetchTagArticleCountsPort fetches tag-level article counts since a given time.
type FetchTagArticleCountsPort interface {
	FetchTagArticleCounts(ctx context.Context, userID uuid.UUID, since time.Time) ([]TagArticleCount, error)
}

// TrendingTag represents a tag that is currently trending (surge in recent articles).
type TrendingTag struct {
	TagName     string
	RecentCount int
	SurgeRatio  float64
}

// TagHotspotPort provides trending tag detection.
type TagHotspotPort interface {
	GetTrendingTags(ctx context.Context, userID uuid.UUID) ([]TrendingTag, error)
}
