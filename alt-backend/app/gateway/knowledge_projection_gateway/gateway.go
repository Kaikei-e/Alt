package knowledge_projection_gateway

import (
	"alt/driver/alt_db"
	"context"
	"fmt"
)

// Gateway implements knowledge projection port interfaces using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new knowledge projection gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// GetProjectionCheckpoint implements knowledge_projection_port.GetProjectionCheckpointPort.
func (g *Gateway) GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error) {
	if g.repo == nil {
		return 0, fmt.Errorf("GetProjectionCheckpoint: database connection not available")
	}
	return g.repo.GetProjectionCheckpoint(ctx, projectorName)
}

// UpdateProjectionCheckpoint implements knowledge_projection_port.UpdateProjectionCheckpointPort.
func (g *Gateway) UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error {
	if g.repo == nil {
		return fmt.Errorf("UpdateProjectionCheckpoint: database connection not available")
	}
	return g.repo.UpdateProjectionCheckpoint(ctx, projectorName, lastSeq)
}
