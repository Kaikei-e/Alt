package knowledge_trail_port

import (
	"context"

	"alt/domain"

	"github.com/google/uuid"
)

// GetTrailPort fetches the user's footprint spine from the knowledge authority.
type GetTrailPort interface {
	GetTrailFootprints(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]domain.TrailFootprint, string, bool, error)
}
