package knowledge_reproject_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

// CreateReprojectRunPort creates a new reproject run.
type CreateReprojectRunPort interface {
	CreateReprojectRun(ctx context.Context, run *domain.ReprojectRun) error
}

// GetReprojectRunPort retrieves a reproject run by ID.
type GetReprojectRunPort interface {
	GetReprojectRun(ctx context.Context, runID uuid.UUID) (*domain.ReprojectRun, error)
}

// UpdateReprojectRunPort updates a reproject run.
type UpdateReprojectRunPort interface {
	UpdateReprojectRun(ctx context.Context, run *domain.ReprojectRun) error
}

// ListReprojectRunsPort lists reproject runs with optional filter.
type ListReprojectRunsPort interface {
	ListReprojectRuns(ctx context.Context, statusFilter string, limit int) ([]domain.ReprojectRun, error)
}

// CompareProjectionsPort compares two projection versions.
type CompareProjectionsPort interface {
	CompareProjections(ctx context.Context, fromVersion, toVersion string) (*domain.ReprojectDiffSummary, error)
}

// CreateProjectionAuditPort stores an audit result.
type CreateProjectionAuditPort interface {
	CreateProjectionAudit(ctx context.Context, audit *domain.ProjectionAudit) error
}

// ListProjectionAuditsPort lists audit results.
type ListProjectionAuditsPort interface {
	ListProjectionAudits(ctx context.Context, projectionName string, limit int) ([]domain.ProjectionAudit, error)
}
