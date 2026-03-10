package usecase

import (
	"context"
	"search-indexer/port"
)

// IndexRecapsUsecase handles indexing recap genres into Meilisearch.
type IndexRecapsUsecase struct {
	recapRepo         port.RecapRepository
	recapSearchEngine port.RecapSearchEngine
}

// RecapIndexResult contains the result of a recap indexing operation.
type RecapIndexResult struct {
	IndexedCount int
	HasMore      bool
	LastSince    string // RFC3339 timestamp for next incremental fetch
}

// NewIndexRecapsUsecase creates a new index recaps usecase.
func NewIndexRecapsUsecase(recapRepo port.RecapRepository, recapSearchEngine port.RecapSearchEngine) *IndexRecapsUsecase {
	return &IndexRecapsUsecase{
		recapRepo:         recapRepo,
		recapSearchEngine: recapSearchEngine,
	}
}

// ExecuteBackfill fetches recap genres and indexes them, paginating via since cursor.
func (u *IndexRecapsUsecase) ExecuteBackfill(ctx context.Context, since string, batchSize int) (*RecapIndexResult, error) {
	docs, hasMore, err := u.recapRepo.GetIndexableGenres(ctx, since, batchSize)
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return &RecapIndexResult{IndexedCount: 0, HasMore: false}, nil
	}

	if err := u.recapSearchEngine.IndexRecapDocuments(ctx, docs); err != nil {
		return nil, err
	}

	// Track the latest executed_at for incremental mark
	lastSince := ""
	for _, d := range docs {
		if d.ExecutedAt > lastSince {
			lastSince = d.ExecutedAt
		}
	}

	return &RecapIndexResult{
		IndexedCount: len(docs),
		HasMore:      hasMore,
		LastSince:    lastSince,
	}, nil
}

// ExecuteIncremental fetches new recap genres since the given timestamp.
func (u *IndexRecapsUsecase) ExecuteIncremental(ctx context.Context, since string, batchSize int) (*RecapIndexResult, error) {
	docs, hasMore, err := u.recapRepo.GetIndexableGenres(ctx, since, batchSize)
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return &RecapIndexResult{IndexedCount: 0, HasMore: false, LastSince: since}, nil
	}

	if err := u.recapSearchEngine.IndexRecapDocuments(ctx, docs); err != nil {
		return nil, err
	}

	lastSince := since
	for _, d := range docs {
		if d.ExecutedAt > lastSince {
			lastSince = d.ExecutedAt
		}
	}

	return &RecapIndexResult{
		IndexedCount: len(docs),
		HasMore:      hasMore,
		LastSince:    lastSince,
	}, nil
}
