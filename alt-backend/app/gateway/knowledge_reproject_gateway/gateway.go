package knowledge_reproject_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Gateway implements reproject and audit port interfaces using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new knowledge reproject gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// CreateReprojectRun implements knowledge_reproject_port.CreateReprojectRunPort.
func (g *Gateway) CreateReprojectRun(ctx context.Context, run *domain.ReprojectRun) error {
	if g.repo == nil {
		return fmt.Errorf("CreateReprojectRun: database connection not available")
	}
	return g.repo.CreateReprojectRun(ctx, run)
}

// GetReprojectRun implements knowledge_reproject_port.GetReprojectRunPort.
func (g *Gateway) GetReprojectRun(ctx context.Context, runID uuid.UUID) (*domain.ReprojectRun, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("GetReprojectRun: database connection not available")
	}
	return g.repo.GetReprojectRun(ctx, runID)
}

// UpdateReprojectRun implements knowledge_reproject_port.UpdateReprojectRunPort.
func (g *Gateway) UpdateReprojectRun(ctx context.Context, run *domain.ReprojectRun) error {
	if g.repo == nil {
		return fmt.Errorf("UpdateReprojectRun: database connection not available")
	}
	return g.repo.UpdateReprojectRun(ctx, run)
}

// ListReprojectRuns implements knowledge_reproject_port.ListReprojectRunsPort.
func (g *Gateway) ListReprojectRuns(ctx context.Context, statusFilter string, limit int) ([]domain.ReprojectRun, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("ListReprojectRuns: database connection not available")
	}
	return g.repo.ListReprojectRuns(ctx, statusFilter, limit)
}

// CompareProjections implements knowledge_reproject_port.CompareProjectionsPort.
func (g *Gateway) CompareProjections(ctx context.Context, fromVersion, toVersion string) (*domain.ReprojectDiffSummary, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("CompareProjections: database connection not available")
	}
	return g.repo.CompareProjections(ctx, fromVersion, toVersion)
}

// CreateProjectionAudit implements knowledge_reproject_port.CreateProjectionAuditPort.
func (g *Gateway) CreateProjectionAudit(ctx context.Context, audit *domain.ProjectionAudit) error {
	if g.repo == nil {
		return fmt.Errorf("CreateProjectionAudit: database connection not available")
	}
	return g.repo.CreateProjectionAudit(ctx, audit)
}

// ListProjectionAudits implements knowledge_reproject_port.ListProjectionAuditsPort.
func (g *Gateway) ListProjectionAudits(ctx context.Context, projectionName string, limit int) ([]domain.ProjectionAudit, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("ListProjectionAudits: database connection not available")
	}
	return g.repo.ListProjectionAudits(ctx, projectionName, limit)
}

// GetProjectionLag implements knowledge_slo_port.GetProjectionLagPort.
func (g *Gateway) GetProjectionLag(ctx context.Context) (time.Duration, error) {
	if g.repo == nil {
		return 0, fmt.Errorf("GetProjectionLag: database connection not available")
	}
	return g.repo.GetProjectionLag(ctx)
}
