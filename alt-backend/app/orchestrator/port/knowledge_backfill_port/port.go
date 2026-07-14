package knowledge_backfill_port

import (
	"alt/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

// CreateBackfillJobPort creates a new backfill job.
type CreateBackfillJobPort interface {
	CreateBackfillJob(ctx context.Context, job domain.KnowledgeBackfillJob) error
}

// GetBackfillJobPort retrieves a backfill job by ID.
type GetBackfillJobPort interface {
	GetBackfillJob(ctx context.Context, jobID uuid.UUID) (*domain.KnowledgeBackfillJob, error)
}

// UpdateBackfillJobPort updates an existing backfill job.
type UpdateBackfillJobPort interface {
	UpdateBackfillJob(ctx context.Context, job domain.KnowledgeBackfillJob) error
}

// ListBackfillJobsPort lists backfill jobs.
type ListBackfillJobsPort interface {
	ListBackfillJobs(ctx context.Context) ([]domain.KnowledgeBackfillJob, error)
}

// ListBackfillArticlesPort lists historical articles in ascending order for replay.
type ListBackfillArticlesPort interface {
	ListBackfillArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error)
}

// CountBackfillArticlesPort counts replayable historical articles.
type CountBackfillArticlesPort interface {
	CountBackfillArticles(ctx context.Context) (int, error)
}

// ListBackfillSummaryTitlesPort lists (summary_version, article) pairs in
// ascending (generated_at, summary_version_id) order for the
// SummaryNarrativeBackfillJob (ADR-000846). Each row carries the article's
// current title — a snapshot at backfill time, NOT title-at-event-time —
// because the articles table is mutable and no article_versions snapshot
// table exists yet.
type ListBackfillSummaryTitlesPort interface {
	ListBackfillSummaryTitles(ctx context.Context, lastGeneratedAt *time.Time, lastSummaryVersionID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillSummaryTitle, error)
}

// CountBackfillSummaryTitlesPort counts (summary_version, article) pairs that
// the SummaryNarrativeBackfillJob will emit a discovered event for.
type CountBackfillSummaryTitlesPort interface {
	CountBackfillSummaryTitles(ctx context.Context) (int, error)
}
