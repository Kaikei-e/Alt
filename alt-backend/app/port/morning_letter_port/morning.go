package morning_letter_port

import (
	"context"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

// MorningRepository defines the interface for accessing morning article groups.
type MorningRepository interface {
	GetMorningArticleGroups(ctx context.Context, since time.Time) ([]*domain.MorningArticleGroup, error)
}

// MorningUsecase defines the interface for the morning letter business logic (overnight updates).
type MorningUsecase interface {
	GetOvernightUpdates(ctx context.Context, userID string) ([]*domain.MorningUpdate, error)
}

// MorningLetterRepository defines data access for Morning Letter documents (via recap-worker REST).
type MorningLetterRepository interface {
	GetLatestLetter(ctx context.Context) (*domain.MorningLetterDocument, error)
	GetLetterByDate(ctx context.Context, targetDate string) (*domain.MorningLetterDocument, error)
	GetLetterSources(ctx context.Context, letterID string) ([]*domain.MorningLetterSourceEntry, error)
	RegenerateLatest(ctx context.Context, editionTimezone string) (*domain.MorningLetterDocument, error)
}

// MorningLetterUsecase defines business logic for reading Morning Letters with subscription filtering.
type MorningLetterUsecase interface {
	GetLatestLetter(ctx context.Context) (*domain.MorningLetterDocument, error)
	GetLetterByDate(ctx context.Context, targetDate string) (*domain.MorningLetterDocument, error)
	GetLetterSources(ctx context.Context, letterID string) ([]*domain.MorningLetterSourceEntry, error)
	// RegenerateLatest triggers an on-demand editorial projection for the
	// caller. Returns (doc, regenerated, retryAfter) where regenerated=false
	// means the caller is rate-limited and retryAfter > 0.
	RegenerateLatest(ctx context.Context, userID, editionTimezone string) (doc *domain.MorningLetterDocument, regenerated bool, retryAfter time.Duration, err error)
	// GetLetterEnrichment returns per-article enrichment for a Letter's
	// sources so the frontend can render each source as a navigable card
	// (article link, tags, related articles, Acolyte chat seed, summary
	// excerpt). Capped server-side to avoid fan-out storms.
	GetLetterEnrichment(ctx context.Context, letterID, userID string) ([]*domain.MorningLetterBulletEnrichment, error)
}

// ArticleMetadataBatchPort batch-fetches articles by id — minimum shape
// the enrichment usecase needs. Implemented over the existing
// AltDBRepository.FetchArticlesByIDs in a gateway wrapper.
type ArticleMetadataBatchPort interface {
	FetchArticlesByIDs(ctx context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error)
}

// FeedTitleBatchPort resolves feed_id -> feed.title in a single round-trip.
type FeedTitleBatchPort interface {
	FetchFeedTitlesByIDs(ctx context.Context, feedIDs []uuid.UUID) (map[uuid.UUID]string, error)
}

// SearchRelatedArticlesPort is a narrow search-indexer interface scoped to
// the related-articles lookup (to avoid importing the full SearchIndexerPort).
type SearchRelatedArticlesPort interface {
	SearchArticles(ctx context.Context, query string, userID string) ([]domain.SearchIndexerArticleHit, error)
}
