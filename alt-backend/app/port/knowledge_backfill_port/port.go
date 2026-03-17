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
