package knowledge_projection_version_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"
)

// Gateway implements projection version port interfaces using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new knowledge projection version gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// GetActiveVersion implements knowledge_projection_version_port.GetActiveVersionPort.
func (g *Gateway) GetActiveVersion(ctx context.Context) (*domain.KnowledgeProjectionVersion, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("GetActiveVersion: database connection not available")
	}
	return g.repo.GetActiveProjectionVersion(ctx)
}

// ListVersions implements knowledge_projection_version_port.ListVersionsPort.
func (g *Gateway) ListVersions(ctx context.Context) ([]domain.KnowledgeProjectionVersion, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("ListVersions: database connection not available")
	}
	return g.repo.ListProjectionVersions(ctx)
}

// CreateVersion implements knowledge_projection_version_port.CreateVersionPort.
func (g *Gateway) CreateVersion(ctx context.Context, version domain.KnowledgeProjectionVersion) error {
	if g.repo == nil {
		return fmt.Errorf("CreateVersion: database connection not available")
	}
	return g.repo.CreateProjectionVersion(ctx, version)
}

// ActivateVersion implements knowledge_projection_version_port.ActivateVersionPort.
func (g *Gateway) ActivateVersion(ctx context.Context, version int) error {
	if g.repo == nil {
		return fmt.Errorf("ActivateVersion: database connection not available")
	}
	return g.repo.ActivateProjectionVersion(ctx, version)
}
