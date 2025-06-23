package repository

import (
	"context"
	"net/url"
	"pre-processor/models"
	"time"
)

//go:generate mockgen -source=interfaces.go -destination=../test/mocks/repository_mocks.go -package=mocks

// ArticleRepository handles article data persistence
type ArticleRepository interface {
	Create(ctx context.Context, article *models.Article) error
	CheckExists(ctx context.Context, urls []string) (bool, error)
	FindForSummarization(ctx context.Context, cursor *Cursor, limit int) ([]*models.Article, *Cursor, error)
	HasUnsummarizedArticles(ctx context.Context) (bool, error)
}

// FeedRepository handles feed data persistence
type FeedRepository interface {
	GetUnprocessedFeeds(ctx context.Context, cursor *Cursor, limit int) ([]*url.URL, *Cursor, error)
	GetProcessingStats(ctx context.Context) (*ProcessingStats, error)
}

// SummaryRepository handles article summary persistence
type SummaryRepository interface {
	Create(ctx context.Context, summary *models.ArticleSummary) error
	FindArticlesWithSummaries(ctx context.Context, cursor *Cursor, limit int) ([]*models.ArticleWithSummary, *Cursor, error)
	Delete(ctx context.Context, summaryID string) error
	Exists(ctx context.Context, summaryID string) (bool, error)
}

// ExternalAPIRepository handles external API calls
type ExternalAPIRepository interface {
	SummarizeArticle(ctx context.Context, article *models.Article) (*models.SummarizedContent, error)
	CheckHealth(ctx context.Context, serviceURL string) error
}

// Cursor represents pagination cursor for efficient pagination
type Cursor struct {
	LastCreatedAt *time.Time
	LastID        string
}

// ProcessingStats represents processing statistics
type ProcessingStats struct {
	TotalFeeds     int
	ProcessedFeeds int
	RemainingFeeds int
}
