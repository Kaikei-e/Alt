package dashboard_usecase

import (
	"context"

	"alt/domain"
	"alt/port/recap_job_port"
)

type GetRecapJobsUsecase interface {
	Execute(ctx context.Context, windowSeconds int64, limit int64) ([]domain.RecapJob, error)
}

type getRecapJobsUsecase struct {
	recapRepo recap_job_port.RecapJobRepository
}

func NewGetRecapJobsUsecase(recapRepo recap_job_port.RecapJobRepository) GetRecapJobsUsecase {
	return &getRecapJobsUsecase{
		recapRepo: recapRepo,
	}
}

func (u *getRecapJobsUsecase) Execute(ctx context.Context, windowSeconds int64, limit int64) ([]domain.RecapJob, error) {
	return u.recapRepo.GetRecapJobs(ctx, windowSeconds, limit)
}
