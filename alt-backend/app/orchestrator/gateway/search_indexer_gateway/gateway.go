package search_indexer_gateway

import (
	"context"

	"alt/domain"
	"alt/orchestrator/driver/search_indexer_connect"
	"alt/orchestrator/port/search_indexer_port"
)

// SearchIndexerGateway adapts search_indexer_connect.Client to search_indexer_port.SearchIndexerPort.
type SearchIndexerGateway struct {
	driver *search_indexer_connect.Client
}

// NewSearchIndexerGateway creates a new SearchIndexerGateway.
func NewSearchIndexerGateway(driver *search_indexer_connect.Client) search_indexer_port.SearchIndexerPort {
	return &SearchIndexerGateway{driver: driver}
}

// SearchArticles searches for articles matching the query.
func (g *SearchIndexerGateway) SearchArticles(ctx context.Context, query string, userID string) ([]domain.SearchIndexerArticleHit, error) {
	hits, err := g.driver.SearchArticles(ctx, query, userID)
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

// SearchArticlesWithPagination searches for articles with pagination support.
func (g *SearchIndexerGateway) SearchArticlesWithPagination(ctx context.Context, query string, userID string, offset int, limit int) ([]domain.SearchIndexerArticleHit, int64, error) {
	hits, total, err := g.driver.SearchArticlesWithPagination(ctx, query, userID, offset, limit)
	if err != nil {
		return nil, 0, err
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
	return domainHits, total, nil
}

// SearchRecapsByTag searches recap genres by tag name.
func (g *SearchIndexerGateway) SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]*domain.RecapSearchResult, error) {
	hits, err := g.driver.SearchRecapsByTag(ctx, tagName, limit)
	if err != nil {
		return nil, err
	}
	results := make([]*domain.RecapSearchResult, len(hits))
	for i, hit := range hits {
		results[i] = &domain.RecapSearchResult{
			JobID:      hit.JobID,
			ExecutedAt: hit.ExecutedAt,
			WindowDays: hit.WindowDays,
			Genre:      hit.Genre,
			Summary:    hit.Summary,
			TopTerms:   hit.TopTerms,
			Tags:       hit.Tags,
			Bullets:    hit.Bullets,
		}
	}
	return results, nil
}

// SearchRecapsByQuery searches recap genres by free-text query.
func (g *SearchIndexerGateway) SearchRecapsByQuery(ctx context.Context, query string, limit int) ([]*domain.RecapSearchResult, int64, error) {
	hits, total, err := g.driver.SearchRecapsByQuery(ctx, query, limit)
	if err != nil {
		return nil, 0, err
	}
	results := make([]*domain.RecapSearchResult, len(hits))
	for i, hit := range hits {
		results[i] = &domain.RecapSearchResult{
			JobID:      hit.JobID,
			ExecutedAt: hit.ExecutedAt,
			WindowDays: hit.WindowDays,
			Genre:      hit.Genre,
			Summary:    hit.Summary,
			TopTerms:   hit.TopTerms,
			Tags:       hit.Tags,
			Bullets:    hit.Bullets,
		}
	}
	return results, total, nil
}
