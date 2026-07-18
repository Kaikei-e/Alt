// Package search_trail_port defines the trail-search narrowing lookup (Wave 9
// — trail search, D25).
package search_trail_port

import (
	"context"

	"alt/domain"

	"github.com/google/uuid"
)

// SearchTrailPort narrows the user's derived episode spine to episodes
// containing at least one footprint whose item_key is in itemKeys. It reuses
// the same sovereign GetTrailFootprints RPC as knowledge_trail_port.GetTrailPort,
// narrowed server-side via filter_item_keys — there is no cursor because the
// caller supplies a single generously-sized limit and pages the search hits,
// not the sovereign call.
type SearchTrailPort interface {
	SearchTrailFootprints(ctx context.Context, userID uuid.UUID, itemKeys []string, limit int) ([]domain.TrailEpisode, error)
}
