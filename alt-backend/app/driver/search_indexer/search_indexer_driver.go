package search_indexer

import (
	"alt/domain"
	"alt/port/search_indexer_port"
	"context"
	"fmt"
)

type HTTPSearchIndexerDriver struct{}

func NewHTTPSearchIndexerDriver() search_indexer_port.SearchIndexerPort {
	return &HTTPSearchIndexerDriver{}
}

func (d *HTTPSearchIndexerDriver) SearchArticles(ctx context.Context, query string, userID string) ([]domain.SearchIndexerArticleHit, error) {
	hits, err := SearchArticlesWithUserID(ctx, query, userID)
	if err != nil {
		return nil, err
	}

	domainHits := make([]domain.SearchIndexerArticleHit, len(hits))
	for i, hit := range hits {
		domainHits[i] = domain.SearchIndexerArticleHit{
			ID:      hit.ID,
			Title:   hit.Title,
			Content: hit.Content,
			Tags:    hit.Tags,
		}
	}

	return domainHits, nil
}

// SearchArticlesWithPagination is a fallback implementation that uses the REST API.
// Note: The REST API v1/search doesn't support pagination, so this returns all results
// and the estimated total is set to the number of hits returned.
// For proper pagination support, use the Connect-RPC driver instead.
func (d *HTTPSearchIndexerDriver) SearchArticlesWithPagination(ctx context.Context, query string, userID string, offset int, limit int) ([]domain.SearchIndexerArticleHit, int64, error) {
	hits, err := d.SearchArticles(ctx, query, userID)
	if err != nil {
		return nil, 0, err
	}

	// Apply pagination locally since REST API doesn't support it
	totalCount := int64(len(hits))
	start := offset
	if start > len(hits) {
		start = len(hits)
	}
	end := start + limit
	if end > len(hits) {
		end = len(hits)
	}

	if start < len(hits) {
		return hits[start:end], totalCount, nil
	}
	return []domain.SearchIndexerArticleHit{}, totalCount, nil
}

// SearchRecapsByTag is not supported by the REST driver.
// Use the Connect-RPC driver for recap search via Meilisearch.
func (d *HTTPSearchIndexerDriver) SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]*domain.RecapSearchResult, error) {
	return nil, fmt.Errorf("SearchRecapsByTag not supported by HTTP driver, use Connect-RPC driver")
}

// SearchRecapsByQuery is not supported by the REST driver.
// Use the Connect-RPC driver for recap search via Meilisearch.
func (d *HTTPSearchIndexerDriver) SearchRecapsByQuery(ctx context.Context, query string, limit int) ([]*domain.RecapSearchResult, int64, error) {
	return nil, 0, fmt.Errorf("SearchRecapsByQuery not supported by HTTP driver, use Connect-RPC driver")
}
