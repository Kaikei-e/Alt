package usecase

import (
	"context"
	"fmt"
	"search-indexer/domain"
	"search-indexer/port"
	"strings"
)

type SearchArticlesWithFiltersUsecase struct {
	searchEngine port.SearchEngine
}

func NewSearchArticlesWithFiltersUsecase(searchEngine port.SearchEngine) *SearchArticlesWithFiltersUsecase {
	return &SearchArticlesWithFiltersUsecase{
		searchEngine: searchEngine,
	}
}

func (u *SearchArticlesWithFiltersUsecase) Execute(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
	// Validate input parameters
	if err := u.validateInput(query, filters, limit); err != nil {
		return nil, err
	}

	// Validate filter tags for security
	if err := domain.ValidateFilterTags(filters); err != nil {
		return nil, fmt.Errorf("invalid filter tags: %w", err)
	}

	// Execute search with filters
	results, err := u.searchEngine.SearchWithFilters(ctx, query, filters, limit)
	if err != nil {
		return nil, fmt.Errorf("search with filters failed: %w", err)
	}

	return results, nil
}

func (u *SearchArticlesWithFiltersUsecase) validateInput(query string, filters []string, limit int) error {
	// Validate query
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query cannot be empty")
	}

	// Validate query length
	if len(query) > 1000 {
		return fmt.Errorf("query too long: maximum 1000 characters, got %d", len(query))
	}

	// Validate limit
	if limit <= 0 {
		return fmt.Errorf("limit must be positive: got %d", limit)
	}

	if limit > 100 {
		return fmt.Errorf("limit too large: maximum 100, got %d", limit)
	}

	// Validate filters (basic validation, detailed validation in search_engine.ValidateFilterTags)
	if len(filters) > 10 {
		return fmt.Errorf("too many filters: maximum 10, got %d", len(filters))
	}

	return nil
}
