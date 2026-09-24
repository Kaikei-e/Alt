package recap_gateway

import (
	"context"

	"alt/domain"
	"alt/orchestrator/driver/recap_job_driver"
	"alt/orchestrator/port/recap_job_port"
)

// RecapJobGateway adapts recap_job_driver.Driver to recap_job_port.RecapJobRepository.
type RecapJobGateway struct {
	driver *recap_job_driver.Driver
}

// NewRecapJobGateway creates a new RecapJobGateway.
func NewRecapJobGateway(driver *recap_job_driver.Driver) recap_job_port.RecapJobRepository {
	return &RecapJobGateway{
		driver: driver,
	}
}

// GetRecapJobs fetches recap jobs via the driver and maps DTOs to domain models.
func (g *RecapJobGateway) GetRecapJobs(ctx context.Context, windowSeconds int64, limit int64) ([]domain.RecapJob, error) {
	dtos, err := g.driver.FetchRecapJobs(ctx, windowSeconds, limit)
	if err != nil {
		return nil, err
	}

	jobs := make([]domain.RecapJob, len(dtos))
	for i, d := range dtos {
		jobs[i] = domain.RecapJob{
			JobID:     d.JobID,
			Status:    d.Status,
			LastStage: d.LastStage,
			KickedAt:  d.KickedAt,
			UpdatedAt: d.UpdatedAt,
		}
	}

	return jobs, nil
}
