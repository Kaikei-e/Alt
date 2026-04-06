// Package search_indexer_connect provides Connect-RPC client for search-indexer service.
package search_indexer_connect

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	"alt/domain"
	searchv2 "alt/gen/proto/services/search/v2"
	"alt/gen/proto/services/search/v2/searchv2connect"
	"alt/port/search_indexer_port"
)

// ConnectSearchIndexerDriver implements SearchIndexerPort using Connect-RPC.
type ConnectSearchIndexerDriver struct {
	client searchv2connect.SearchServiceClient
}

// NewConnectSearchIndexerDriver creates a new Connect-RPC client for search-indexer.
func NewConnectSearchIndexerDriver(baseURL string) search_indexer_port.SearchIndexerPort {
	client := searchv2connect.NewSearchServiceClient(
		http.DefaultClient,
		baseURL,
	)
	return &ConnectSearchIndexerDriver{client: client}
}

// SearchArticles searches for articles matching the query via Connect-RPC.
func (d *ConnectSearchIndexerDriver) SearchArticles(ctx context.Context, query string, userID string) ([]domain.SearchIndexerArticleHit, error) {
	resp, err := d.client.SearchArticles(ctx, connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  query,
		UserId: userID,
		Limit:  20,
	}))
	if err != nil {
		return nil, err
	}

	// Convert to domain model
	hits := make([]domain.SearchIndexerArticleHit, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		hits[i] = domain.SearchIndexerArticleHit{
			ID:      hit.Id,
			Title:   hit.Title,
			Content: hit.Content,
			Tags:    hit.Tags,
		}
	}

	return hits, nil
}

// SearchArticlesWithPagination searches for articles with pagination support via Connect-RPC.
func (d *ConnectSearchIndexerDriver) SearchArticlesWithPagination(ctx context.Context, query string, userID string, offset int, limit int) ([]domain.SearchIndexerArticleHit, int64, error) {
	resp, err := d.client.SearchArticles(ctx, connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  query,
		UserId: userID,
		Offset: int32(offset),
		Limit:  int32(limit),
	}))
	if err != nil {
		return nil, 0, err
	}

	// Convert to domain model
	hits := make([]domain.SearchIndexerArticleHit, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		hits[i] = domain.SearchIndexerArticleHit{
			ID:      hit.Id,
			Title:   hit.Title,
			Content: hit.Content,
			Tags:    hit.Tags,
		}
	}

	return hits, resp.Msg.EstimatedTotalHits, nil
}

// SearchRecapsByTag searches recap genres by tag name via search-indexer's Meilisearch.
func (d *ConnectSearchIndexerDriver) SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]*domain.RecapSearchResult, error) {
	resp, err := d.client.SearchRecaps(ctx, connect.NewRequest(&searchv2.SearchRecapsRequest{
		TagName: tagName,
		Limit:   int32(limit),
	}))
	if err != nil {
		return nil, err
	}

	results := make([]*domain.RecapSearchResult, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		results[i] = &domain.RecapSearchResult{
			JobID:      hit.JobId,
			ExecutedAt: hit.ExecutedAt,
			WindowDays: int(hit.WindowDays),
			Genre:      hit.Genre,
			Summary:    hit.Summary,
			TopTerms:   hit.TopTerms,
			Tags:       hit.Tags,
			Bullets:    hit.Bullets,
		}
	}

	return results, nil
}

// SearchRecapsByQuery searches recap genres by free-text query via search-indexer's Meilisearch.
func (d *ConnectSearchIndexerDriver) SearchRecapsByQuery(ctx context.Context, query string, limit int) ([]*domain.RecapSearchResult, int64, error) {
	q := &query
	resp, err := d.client.SearchRecaps(ctx, connect.NewRequest(&searchv2.SearchRecapsRequest{
		Query: q,
		Limit: int32(limit),
	}))
	if err != nil {
		return nil, 0, err
	}

	results := make([]*domain.RecapSearchResult, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		results[i] = &domain.RecapSearchResult{
			JobID:      hit.JobId,
			ExecutedAt: hit.ExecutedAt,
			WindowDays: int(hit.WindowDays),
			Genre:      hit.Genre,
			Summary:    hit.Summary,
			TopTerms:   hit.TopTerms,
			Tags:       hit.Tags,
			Bullets:    hit.Bullets,
		}
	}

	return results, resp.Msg.EstimatedTotalHits, nil
}
