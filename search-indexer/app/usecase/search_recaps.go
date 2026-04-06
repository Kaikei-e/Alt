package usecase

import (
	"context"
	"search-indexer/domain"
	"search-indexer/port"
)

// SearchRecapsUsecase handles searching recap documents in Meilisearch.
type SearchRecapsUsecase struct {
	recapSearchEngine port.RecapSearchEngine
}

// SearchRecapsResult contains the search results.
type SearchRecapsResult struct {
	Hits               []domain.RecapDocument
	EstimatedTotalHits int64
}

// NewSearchRecapsUsecase creates a new search recaps usecase.
func NewSearchRecapsUsecase(recapSearchEngine port.RecapSearchEngine) *SearchRecapsUsecase {
	return &SearchRecapsUsecase{recapSearchEngine: recapSearchEngine}
}

// Execute searches recap documents by tag name.
func (u *SearchRecapsUsecase) Execute(ctx context.Context, tagName string, limit int) (*SearchRecapsResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	docs, total, err := u.recapSearchEngine.SearchRecaps(ctx, tagName, limit)
	if err != nil {
		return nil, err
	}

	return &SearchRecapsResult{
		Hits:               docs,
		EstimatedTotalHits: total,
	}, nil
}

// ExecuteByQuery searches recap documents by free-text query.
func (u *SearchRecapsUsecase) ExecuteByQuery(ctx context.Context, query string, limit int) (*SearchRecapsResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	docs, total, err := u.recapSearchEngine.SearchRecaps(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	return &SearchRecapsResult{
		Hits:               docs,
		EstimatedTotalHits: total,
	}, nil
}
