package domain

import (
	"time"
	"github.com/google/uuid"
)

// Feed represents a RSS/Atom feed
type Feed struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Title       string    `json:"title" db:"title"`
	Description string    `json:"description" db:"description"`
	URL         string    `json:"url" db:"url"`          // Feed URL
	Link        string    `json:"link" db:"link"`        // Website URL
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Language    string    `json:"language" db:"language"`
	Category    string    `json:"category" db:"category"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	LastFetchedAt *time.Time `json:"last_fetched_at" db:"last_fetched_at"`
	FetchInterval int      `json:"fetch_interval" db:"fetch_interval"` // in minutes
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Article represents an article from a feed
type Article struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	FeedID      uuid.UUID  `json:"feed_id" db:"feed_id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Title       string     `json:"title" db:"title"`
	Content     string     `json:"content" db:"content"`
	Summary     string     `json:"summary" db:"summary"`
	URL         string     `json:"url" db:"url"`
	Author      string     `json:"author" db:"author"`
	Language    string     `json:"language" db:"language"`
	Tags        []string   `json:"tags" db:"tags"`
	PublishedAt time.Time  `json:"published_at" db:"published_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// UserFeed represents the relationship between users and feeds
type UserFeed struct {
	UserID      uuid.UUID  `json:"user_id" db:"user_id"`
	FeedID      uuid.UUID  `json:"feed_id" db:"feed_id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Category    string     `json:"category" db:"category"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// ReadStatus represents user's reading status for articles
type ReadStatus struct {
	UserID      uuid.UUID  `json:"user_id" db:"user_id"`
	ArticleID   uuid.UUID  `json:"article_id" db:"article_id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	IsRead      bool       `json:"is_read" db:"is_read"`
	IsFavorite  bool       `json:"is_favorite" db:"is_favorite"`
	ReadAt      *time.Time `json:"read_at" db:"read_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// FavoriteFeed represents user's favorite feeds
type FavoriteFeed struct {
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	FeedID    uuid.UUID `json:"feed_id" db:"feed_id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// FeedValidationResult represents the result of feed validation
type FeedValidationResult struct {
	IsValid      bool      `json:"is_valid"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	URL          string    `json:"url"`
	ArticleCount int       `json:"article_count"`
	LastUpdated  time.Time `json:"last_updated"`
	Error        string    `json:"error,omitempty"`
}

// TenantFeedStats represents tenant-specific feed statistics
type TenantFeedStats struct {
	FeedID        uuid.UUID `json:"feed_id"`
	TenantID      uuid.UUID `json:"tenant_id"`
	ArticleCount  int       `json:"article_count"`
	ReaderCount   int       `json:"reader_count"`
	LastFetchedAt time.Time `json:"last_fetched_at"`
}

// CreateFeedRequest represents request to create a new feed
type CreateFeedRequest struct {
	Title       string `json:"title" validate:"required,min=1,max=200"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url" validate:"required,url"`
	Category    string `json:"category,omitempty"`
	Language    string `json:"language,omitempty"`
}

// UpdateFeedRequest represents request to update a feed
type UpdateFeedRequest struct {
	Title       *string `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
	Description *string `json:"description,omitempty"`
	Category    *string `json:"category,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

// FeedFilter represents filter options for feed queries
type FeedFilter struct {
	TenantID     uuid.UUID `json:"tenant_id"`
	UserID       *uuid.UUID `json:"user_id,omitempty"`
	Category     *string   `json:"category,omitempty"`
	IsActive     *bool     `json:"is_active,omitempty"`
	Language     *string   `json:"language,omitempty"`
	SearchQuery  *string   `json:"search_query,omitempty"`
	Limit        int       `json:"limit"`
	Offset       int       `json:"offset"`
}

// ArticleFilter represents filter options for article queries
type ArticleFilter struct {
	TenantID     uuid.UUID  `json:"tenant_id"`
	UserID       *uuid.UUID `json:"user_id,omitempty"`
	FeedID       *uuid.UUID `json:"feed_id,omitempty"`
	IsRead       *bool      `json:"is_read,omitempty"`
	IsFavorite   *bool      `json:"is_favorite,omitempty"`
	Language     *string    `json:"language,omitempty"`
	Tags         []string   `json:"tags,omitempty"`
	PublishedAfter  *time.Time `json:"published_after,omitempty"`
	PublishedBefore *time.Time `json:"published_before,omitempty"`
	SearchQuery  *string    `json:"search_query,omitempty"`
	Limit        int        `json:"limit"`
	Offset       int        `json:"offset"`
}