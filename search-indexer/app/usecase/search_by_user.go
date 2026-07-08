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
	if userID == "" {
		return nil, errors.New("user_id parameter required")
	}
	// Same length/control-character/zero-width validation and NFC
	// sanitization as SearchArticlesUsecase.Execute -- previously this path
	// only checked for an empty query, so a control-character or
	// over-length query rejected by the unfiltered search endpoint would be
	// silently accepted here (see MED finding on validation-inconsistency).
	sanitizedQuery, err := validateAndSanitizeQuery(query, 20)
	if err != nil {
		return nil, err
	}

	docs, err := u.searchEngine.SearchByUserID(ctx, sanitizedQuery, userID, 20)
	if err != nil {
		return nil, err
	}

	return &SearchByUserResult{
		Query: sanitizedQuery,
		Hits:  docs,
	}, nil
}

// ExecuteWithPagination performs a user-scoped search with pagination.
func (u *SearchByUserUsecase) ExecuteWithPagination(ctx context.Context, query, userID string, offset, limit int64) (*SearchByUserResult, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}
	// limit<=0 means "caller didn't specify one" -- the driver already
	// defaults and clamps it (see SearchByUserIDWithPagination). Validate
	// against that same effective bound rather than the raw (possibly
	// zero) limit, so callers that omit limit aren't rejected here.
	effectiveLimit := limit
	if effectiveLimit <= 0 {
		effectiveLimit = 20
	}
	sanitizedQuery, err := validateAndSanitizeQuery(query, int(effectiveLimit))
	if err != nil {
		return nil, err
	}

	docs, total, err := u.searchEngine.SearchByUserIDWithPagination(ctx, sanitizedQuery, userID, offset, limit)
	if err != nil {
		return nil, err
	}

	return &SearchByUserResult{
		Query:              sanitizedQuery,
		Hits:               docs,
		EstimatedTotalHits: total,
	}, nil
}
