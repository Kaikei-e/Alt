package tag_set_version_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"
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
