package usecase

import (
	"context"
	"errors"
	"search-indexer/domain"
	"search-indexer/port"
)

// SearchByUserUsecase handles user-scoped search operations.
type SearchByUserUsecase struct {
	searchEngine port.SearchEngine
}

func NewSearchByUserUsecase(searchEngine port.SearchEngine) *SearchByUserUsecase {
	return &SearchByUserUsecase{searchEngine: searchEngine}
}

// SearchByUserResult holds the result of a user-scoped search.
type SearchByUserResult struct {
	Query              string
	Hits               []domain.SearchDocument
	EstimatedTotalHits int64
}

// Execute performs a user-scoped search with a fixed limit.
func (u *SearchByUserUsecase) Execute(ctx context.Context, query, userID string) (*SearchByUserResult, error) {
	if query == "" {
		return nil, errors.New("query parameter required")
	}
	if userID == "" {
		return nil, errors.New("user_id parameter required")
	}

	docs, err := u.searchEngine.SearchByUserID(ctx, query, userID, 20)
	if err != nil {
		return nil, err
	}

	return &SearchByUserResult{
		Query: query,
		Hits:  docs,
	}, nil
}

// ExecuteWithPagination performs a user-scoped search with pagination.
func (u *SearchByUserUsecase) ExecuteWithPagination(ctx context.Context, query, userID string, offset, limit int64) (*SearchByUserResult, error) {
	if query == "" {
		return nil, errors.New("query is required")
	}
	if userID == "" {
		return nil, errors.New("user_id is required")
	}

	docs, total, err := u.searchEngine.SearchByUserIDWithPagination(ctx, query, userID, offset, limit)
	if err != nil {
		return nil, err
	}

	return &SearchByUserResult{
		Query:              query,
		Hits:               docs,
		EstimatedTotalHits: total,
	}, nil
}
