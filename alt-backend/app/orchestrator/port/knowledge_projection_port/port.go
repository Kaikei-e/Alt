package knowledge_projection_port

import (
	"context"
)

// GetProjectionCheckpointPort reads the projection checkpoint.
type GetProjectionCheckpointPort interface {
	GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error)
}

// UpdateProjectionCheckpointPort updates the projection checkpoint.
type UpdateProjectionCheckpointPort interface {
	UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error
}
