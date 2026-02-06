package repository

import (
	"context"
	"io"
	"net/url"
	"time"

	"pre-processor/domain"
)

//go:generate mockgen -source=interfaces.go -destination=../test/mocks/repository_mocks.go -package=mocks

// ArticleRepository handles article data persistence.
type ArticleRepository interface {
	Create(ctx context.Context, article *domain.Article) error
	CheckExists(ctx context.Context, urls []string) (bool, error)
	FindForSummarization(ctx context.Context, cursor *domain.Cursor, limit int) ([]*domain.Article, *domain.Cursor, error)
	HasUnsummarizedArticles(ctx context.Context) (bool, error)
	FindByID(ctx context.Context, articleID string) (*domain.Article, error)
	FetchInoreaderArticles(ctx context.Context, since time.Time) ([]*domain.Article, error)
	UpsertArticles(ctx context.Context, articles []*domain.Article) error
}

// FeedRepository handles feed data persistence.
type FeedRepository interface {
	GetUnprocessedFeeds(ctx context.Context, cursor *domain.Cursor, limit int) ([]*url.URL, *domain.Cursor, error)
	GetProcessingStats(ctx context.Context) (*domain.ProcessingStatistics, error)
}

// SummaryRepository handles article summary persistence.
type SummaryRepository interface {
	Create(ctx context.Context, summary *domain.ArticleSummary) error
	FindArticlesWithSummaries(ctx context.Context, cursor *domain.Cursor, limit int) ([]*domain.ArticleWithSummary, *domain.Cursor, error)
	Delete(ctx context.Context, summaryID string) error
	Exists(ctx context.Context, summaryID string) (bool, error)
}

// ExternalAPIRepository handles external API calls.
type ExternalAPIRepository interface {
	SummarizeArticle(ctx context.Context, article *domain.Article, priority string) (*domain.SummarizedContent, error)
	StreamSummarizeArticle(ctx context.Context, article *domain.Article, priority string) (io.ReadCloser, error)
	CheckHealth(ctx context.Context, serviceURL string) error
	GetSystemUserID(ctx context.Context) (string, error)
}

// SummarizeJobRepository handles summarization job queue persistence.
type SummarizeJobRepository interface {
	CreateJob(ctx context.Context, articleID string) (string, error)
	GetJob(ctx context.Context, jobID string) (*domain.SummarizeJob, error)
	UpdateJobStatus(ctx context.Context, jobID string, status domain.SummarizeJobStatus, summary string, errorMessage string) error
	GetPendingJobs(ctx context.Context, limit int) ([]*domain.SummarizeJob, error)
}
