package tag_set_version_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Gateway implements tag set version port interfaces using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new tag set version gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// CreateTagSetVersion implements tag_set_version_port.CreateTagSetVersionPort.
func (g *Gateway) CreateTagSetVersion(ctx context.Context, tsv domain.TagSetVersion) error {
	if g.repo == nil {
		return fmt.Errorf("CreateTagSetVersion: database connection not available")
	}
	return g.repo.CreateTagSetVersion(ctx, tsv)
}

// GetTagSetVersionByID implements tag_set_version_port.GetTagSetVersionByIDPort.
func (g *Gateway) GetTagSetVersionByID(ctx context.Context, tagSetVersionID uuid.UUID) (domain.TagSetVersion, error) {
	if g.repo == nil {
		return domain.TagSetVersion{}, fmt.Errorf("GetTagSetVersionByID: database connection not available")
	}
	return g.repo.GetTagSetVersionByID(ctx, tagSetVersionID)
}

// MarkTagSetVersionSuperseded implements tag_set_version_port.MarkTagSetVersionSupersededPort.
func (g *Gateway) MarkTagSetVersionSuperseded(ctx context.Context, articleID uuid.UUID, newVersionID uuid.UUID) (*domain.TagSetVersion, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("MarkTagSetVersionSuperseded: database connection not available")
	}
	return g.repo.MarkTagSetVersionSuperseded(ctx, articleID, newVersionID)
}
