// Package internal_article_port defines interfaces for internal article API operations.
package internal_article_port

import (
	"context"
	"time"
)

// ArticleWithTags represents an article with its associated tags.
type ArticleWithTags struct {
	ID        string
	Title     string
	Content   string
	Tags      []string
	CreatedAt time.Time
	UserID    string
}

// DeletedArticle represents a deleted article.
type DeletedArticle struct {
	ID        string
	DeletedAt time.Time
}

// ListArticlesWithTagsPort provides backward keyset pagination for articles with tags.
type ListArticlesWithTagsPort interface {
	ListArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*ArticleWithTags, *time.Time, string, error)
}

// ListArticlesWithTagsForwardPort provides forward keyset pagination for articles with tags.
type ListArticlesWithTagsForwardPort interface {
	ListArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*ArticleWithTags, *time.Time, string, error)
}

// ListDeletedArticlesPort provides pagination for deleted articles.
type ListDeletedArticlesPort interface {
	ListDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]*DeletedArticle, *time.Time, error)
}

// GetLatestArticleTimestampPort returns the latest article created_at.
type GetLatestArticleTimestampPort interface {
	GetLatestArticleTimestamp(ctx context.Context) (*time.Time, error)
}

// GetArticleByIDPort retrieves a single article with tags by ID.
type GetArticleByIDPort interface {
	GetArticleByID(ctx context.Context, articleID string) (*ArticleWithTags, error)
}

// ── Phase 2: Article write operations (for pre-processor) ──

// ArticleContent represents article content for summarization.
type ArticleContent struct {
	ID      string
	Title   string
	Content string
	URL     string
}

// CheckArticleExistsPort checks if an article exists by URL and feed.
type CheckArticleExistsPort interface {
	CheckArticleExists(ctx context.Context, url string, feedID string) (exists bool, articleID string, err error)
}

// CreateArticleParams holds parameters for creating an article.
type CreateArticleParams struct {
	Title       string
	URL         string
	Content     string
	FeedID      string
	UserID      string
	PublishedAt time.Time
}

// CreateArticlePort creates a new article.
type CreateArticlePort interface {
	CreateArticle(ctx context.Context, params CreateArticleParams) (articleID string, err error)
}

// SaveArticleSummaryPort saves an article summary.
type SaveArticleSummaryPort interface {
	SaveArticleSummary(ctx context.Context, articleID string, userID string, summary string, language string) error
}

// GetArticleContentPort returns article content for summarization.
type GetArticleContentPort interface {
	GetArticleContent(ctx context.Context, articleID string) (*ArticleContent, error)
}

// ── Phase 4: Summary quality operations (for pre-processor quality checker) ──

// ArticleWithSummaryResult represents an article with its summary for quality checking.
type ArticleWithSummaryResult struct {
	ArticleID       string
	ArticleContent  string
	ArticleURL      string
	SummaryID       string
	SummaryJapanese string
	CreatedAt       time.Time
}

// DeleteArticleSummaryPort deletes an article summary.
type DeleteArticleSummaryPort interface {
	DeleteArticleSummary(ctx context.Context, articleID string) error
}

// CheckArticleSummaryExistsPort checks if an article summary exists.
type CheckArticleSummaryExistsPort interface {
	CheckArticleSummaryExists(ctx context.Context, articleID string) (exists bool, summaryID string, err error)
}

// FindArticlesWithSummariesPort returns articles with summaries.
type FindArticlesWithSummariesPort interface {
	FindArticlesWithSummaries(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*ArticleWithSummaryResult, *time.Time, string, error)
}

// ── Summarization operations (for pre-processor polling) ──

// UnsummarizedArticle represents an article without a summary.
type UnsummarizedArticle struct {
	ID        string
	Title     string
	Content   string
	URL       string
	CreatedAt time.Time
	UserID    string
}

// ListUnsummarizedArticlesPort returns articles without summaries.
type ListUnsummarizedArticlesPort interface {
	ListUnsummarizedArticles(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*UnsummarizedArticle, *time.Time, string, error)
}

// HasUnsummarizedArticlesPort checks if any articles lack summaries.
type HasUnsummarizedArticlesPort interface {
	HasUnsummarizedArticles(ctx context.Context) (bool, error)
}
