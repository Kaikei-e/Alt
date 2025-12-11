package recap_job_port

import (
	"alt/domain"
	"context"
)

type RecapJobRepository interface {
	GetRecapJobs(ctx context.Context, windowSeconds int64, limit int64) ([]domain.RecapJob, error)
}
