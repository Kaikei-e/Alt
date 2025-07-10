package usecase

import (
	"context"
	"errors"
	"search-indexer/domain"
	"search-indexer/port"
	"search-indexer/utils"
)

type SearchArticlesUsecase struct {
	searchEngine port.SearchEngine
	sanitizer    *utils.QuerySanitizer
}

type SearchResult struct {
	Query     string
	Documents []domain.SearchDocument
	Total     int
}

func NewSearchArticlesUsecase(searchEngine port.SearchEngine) *SearchArticlesUsecase {
	return &SearchArticlesUsecase{
		searchEngine: searchEngine,
		sanitizer:    utils.NewQuerySanitizer(utils.DefaultSecurityConfig()),
	}
}

func (u *SearchArticlesUsecase) Execute(ctx context.Context, query string, limit int) (*SearchResult, error) {
	if query == "" {
		return nil, errors.New("query cannot be empty")
	}

	if limit <= 0 {
		return nil, errors.New("limit must be greater than 0")
	}

	if limit > 1000 {
		return nil, errors.New("limit too large")
	}

	// Validate query for security concerns
	if err := u.sanitizer.ValidateQuery(ctx, query); err != nil {
		return nil, err
	}

	// Sanitize query to prevent injection attacks
	sanitizedQuery, err := u.sanitizer.SanitizeQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	// Additional validation after sanitization
	if len(sanitizedQuery) > 1000 {
		return nil, errors.New("query too long")
	}

	// If query becomes empty after sanitization, return empty result
	if sanitizedQuery == "" {
		return &SearchResult{
			Query:     sanitizedQuery,
			Documents: []domain.SearchDocument{},
			Total:     0,
		}, nil
	}

	documents, err := u.searchEngine.Search(ctx, sanitizedQuery, limit)
	if err != nil {
		return nil, err
	}

	return &SearchResult{
		Query:     sanitizedQuery,
		Documents: documents,
		Total:     len(documents),
	}, nil
}
