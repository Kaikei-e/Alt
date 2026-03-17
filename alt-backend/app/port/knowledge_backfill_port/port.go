package knowledge_backfill_port

import (
	"alt/domain"
	"context"

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
