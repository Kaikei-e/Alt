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
	// FetchInoreaderArticlesForEmptyFeeds returns the articles whose feed has no
	// articles yet, plus the fetched_at watermark of the last row the query
	// scanned. The empty-feed filter runs after the LIMIT, so callers must
	// advance their cursor with the watermark and not with the last returned
	// article — otherwise a window where nothing survives the filter is
	// rescanned forever. The watermark is zero when the query scanned no rows.
	FetchInoreaderArticlesForEmptyFeeds(ctx context.Context, fetchedAfter time.Time, limit int) ([]*domain.Article, time.Time, error)
	UpsertArticles(ctx context.Context, articles []*domain.Article) error
	UpsertArticlesWithFeedID(ctx context.Context, articles []*domain.Article) error
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
	Delete(ctx context.Context, articleID string) error
	Exists(ctx context.Context, articleID string) (bool, error)
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
	HasRecentSuccessfulJob(ctx context.Context, articleID string, since time.Time) (bool, error)
	// HasInFlightJob reports whether the article has a pending/running job whose
	// updated_at is newer than the given cutoff. Stale rows are excluded so a
	// crashed worker cannot block re-enqueue indefinitely.
	HasInFlightJob(ctx context.Context, articleID string, since time.Time) (bool, error)
	// HasDeadLetterJob reports whether the article's most recent job row is in
	// the terminal dead_letter status — written only for the explicit
	// domain.ErrContentNotProcessable classification, a genuinely permanent
	// failure. There is no time cutoff, but the check is scoped to the
	// latest row per article_id so a stale dead_letter row cannot out-vote a
	// later, more current row (e.g. after InvalidateCompletedJobSummary).
	HasDeadLetterJob(ctx context.Context, articleID string) (bool, error)
	// HasRecentFailedJob reports whether the article has a retry-exhausted
	// ('failed') job newer than the given cutoff. Unlike HasDeadLetterJob,
	// 'failed' covers retry exhaustion for any error — including transient
	// infrastructure failures — so the block is bounded by since, not
	// permanent.
	HasRecentFailedJob(ctx context.Context, articleID string, since time.Time) (bool, error)
	GetJob(ctx context.Context, jobID string) (*domain.SummarizeJob, error)
	UpdateJobStatus(ctx context.Context, jobID string, status domain.SummarizeJobStatus, summary string, errorMessage string) error
	// CompleteJobWithNotification completes the job and enqueues its
	// summary_ready notification_outbox row in one local transaction, so the
	// notification exists if and only if the completion committed. Callers
	// that have a real summary and a recipient must use this instead of
	// UpdateJobStatus(..., Completed, ...).
	CompleteJobWithNotification(ctx context.Context, jobID, summary, userID, articleID string) error
	GetPendingJobs(ctx context.Context, limit int) ([]*domain.SummarizeJob, error)
	// DequeueJobs atomically selects pending jobs and transitions them to running
	// in a single transaction, preventing duplicate processing under concurrency.
	DequeueJobs(ctx context.Context, limit int) ([]*domain.SummarizeJob, error)
	RecoverStuckJobs(ctx context.Context) (int64, error)
	// InvalidateCompletedJobSummary NULLs out the summary column on completed jobs
	// for the given article, so HasRecentSuccessfulJob no longer blocks re-enqueue.
	// This is the compensating transaction for quality-check deletion.
	InvalidateCompletedJobSummary(ctx context.Context, articleID string) error
}
