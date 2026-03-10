package port

import (
	"context"
	"search-indexer/domain"
)

// RecapRepository provides access to recap data from recap-worker's HTTP API.
type RecapRepository interface {
	// GetIndexableGenres fetches completed recap genres for indexing.
	// If since is non-empty, only returns genres after that timestamp (incremental).
	GetIndexableGenres(ctx context.Context, since string, limit int) ([]domain.RecapDocument, bool, error)
}
