package summary_version_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Gateway implements summary version port interfaces using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new summary version gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// CreateSummaryVersion implements summary_version_port.CreateSummaryVersionPort.
func (g *Gateway) CreateSummaryVersion(ctx context.Context, sv domain.SummaryVersion) error {
	if g.repo == nil {
		return fmt.Errorf("CreateSummaryVersion: database connection not available")
	}
	return g.repo.CreateSummaryVersion(ctx, sv)
}

// GetLatestSummaryVersion implements summary_version_port.GetLatestSummaryVersionPort.
func (g *Gateway) GetLatestSummaryVersion(ctx context.Context, articleID uuid.UUID) (domain.SummaryVersion, error) {
	if g.repo == nil {
		return domain.SummaryVersion{}, fmt.Errorf("GetLatestSummaryVersion: database connection not available")
	}
	return g.repo.GetLatestSummaryVersion(ctx, articleID)
}

// GetSummaryVersionByID implements summary_version_port.GetSummaryVersionByIDPort.
func (g *Gateway) GetSummaryVersionByID(ctx context.Context, summaryVersionID uuid.UUID) (domain.SummaryVersion, error) {
	if g.repo == nil {
		return domain.SummaryVersion{}, fmt.Errorf("GetSummaryVersionByID: database connection not available")
	}
	return g.repo.GetSummaryVersionByID(ctx, summaryVersionID)
}

// MarkSummaryVersionSuperseded implements summary_version_port.MarkSummaryVersionSupersededPort.
func (g *Gateway) MarkSummaryVersionSuperseded(ctx context.Context, articleID uuid.UUID, newVersionID uuid.UUID) (*domain.SummaryVersion, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("MarkSummaryVersionSuperseded: database connection not available")
	}
	return g.repo.MarkSummaryVersionSuperseded(ctx, articleID, newVersionID)
}
