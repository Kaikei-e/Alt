package usecase

import (
	"context"
	"errors"
	"search-indexer/domain"
	"search-indexer/port"
)

type SearchArticlesUsecase struct {
	searchEngine port.SearchEngine
}

type SearchResult struct {
	Query     string
	Documents []domain.SearchDocument
	Total     int
}

func NewSearchArticlesUsecase(searchEngine port.SearchEngine) *SearchArticlesUsecase {
	return &SearchArticlesUsecase{
		searchEngine: searchEngine,
	}
}

func (u *SearchArticlesUsecase) Execute(ctx context.Context, query string, limit int) (*SearchResult, error) {
	if query == "" {
		return nil, errors.New("query cannot be empty")
	}

	if limit <= 0 {
		return nil, errors.New("limit must be greater than 0")
	}

	if len(query) > 1000 {
		return nil, errors.New("query too long")
	}

	if limit > 1000 {
		return nil, errors.New("limit too large")
	}

	documents, err := u.searchEngine.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	return &SearchResult{
		Query:     query,
		Documents: documents,
		Total:     len(documents),
	}, nil
}
