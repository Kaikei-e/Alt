package domain

import "context"

// SearchResult represents a single hit from the search engine.
type SearchHit struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

// SearchClient defines the interface for searching external indices (e.g. Meilisearch).
type SearchClient interface {
	Search(ctx context.Context, query string) ([]SearchHit, error)
}
