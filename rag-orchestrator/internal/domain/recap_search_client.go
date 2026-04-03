package domain

import "context"

// RecapSearchResult represents a recap genre matching a tag search.
type RecapSearchResult struct {
	Genre    string
	Summary  string
	TopTerms []string
}

// RecapSearchClient searches recap summaries by tag.
type RecapSearchClient interface {
	SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]RecapSearchResult, error)
}
