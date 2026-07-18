package knowledge_trail_port

import (
	"context"

	"alt/domain"

	"github.com/google/uuid"
)

// GetTrailPort fetches the user's footprint spine from the knowledge authority.
// Episodes are the spine's default display unit (D24/D30, Wave 8); footprints
// is the legacy flat spine, superseded and empty once episodes ship.
type GetTrailPort interface {
	GetTrailFootprints(ctx context.Context, userID uuid.UUID, cursor string, limit int, filterTags []string) ([]domain.TrailFootprint, []domain.TrailBranch, []domain.TrailEpisode, string, bool, error)
}
