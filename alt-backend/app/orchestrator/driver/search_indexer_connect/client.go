// Package search_indexer_connect provides Connect-RPC client for search-indexer service.
package search_indexer_connect

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	searchv2 "alt/gen/proto/services/search/v2"
	"alt/gen/proto/services/search/v2/searchv2connect"
	"alt/utils/safeconv"
)

// ArticleHit represents a raw search hit from search-indexer.
type ArticleHit struct {
	ID      string
	Title   string
	Content string
	Tags    []string
}

// RecapHit represents a raw recap search result from search-indexer.
type RecapHit struct {
	JobID      string
	ExecutedAt string
	WindowDays int
	Genre      string
	Summary    string
	TopTerms   []string
	Tags       []string
	Bullets    []string
}

// Client provides Connect-RPC client for search-indexer.
type Client struct {
	client searchv2connect.SearchServiceClient
}

// NewClient creates a new Connect-RPC client for search-indexer.
// Auth is mTLS at the transport layer, and WithProtoJSON is used so contract tooling can read the wire format.
func NewClient(baseURL string) *Client {
	client := searchv2connect.NewSearchServiceClient(
		http.DefaultClient,
		baseURL,
		connect.WithProtoJSON(),
	)
	return &Client{client: client}
}

// SearchArticles searches for articles matching the query via Connect-RPC.
func (d *Client) SearchArticles(ctx context.Context, query string, userID string) ([]ArticleHit, error) {
	resp, err := d.client.SearchArticles(ctx, connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  query,
		UserId: userID,
		Limit:  20,
	}))
	if err != nil {
		return nil, err
	}

	hits := make([]ArticleHit, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		hits[i] = ArticleHit{
			ID:      hit.Id,
			Title:   hit.Title,
			Content: hit.Content,
			Tags:    hit.Tags,
		}
	}

	return hits, nil
}

// SearchArticlesWithPagination searches for articles with pagination support via Connect-RPC.
func (d *Client) SearchArticlesWithPagination(ctx context.Context, query string, userID string, offset int, limit int) ([]ArticleHit, int64, error) {
	resp, err := d.client.SearchArticles(ctx, connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  query,
		UserId: userID,
		Offset: safeconv.Int32(offset),
		Limit:  safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, 0, err
	}

	hits := make([]ArticleHit, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		hits[i] = ArticleHit{
			ID:      hit.Id,
			Title:   hit.Title,
			Content: hit.Content,
			Tags:    hit.Tags,
		}
	}

	return hits, resp.Msg.EstimatedTotalHits, nil
}

// SearchRecapsByTag searches recap genres by tag name via search-indexer's Meilisearch.
func (d *Client) SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]RecapHit, error) {
	resp, err := d.client.SearchRecaps(ctx, connect.NewRequest(&searchv2.SearchRecapsRequest{
		TagName: tagName,
		Limit:   safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, err
	}

	results := make([]RecapHit, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		results[i] = RecapHit{
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
func (d *Client) SearchRecapsByQuery(ctx context.Context, query string, limit int) ([]RecapHit, int64, error) {
	q := &query
	resp, err := d.client.SearchRecaps(ctx, connect.NewRequest(&searchv2.SearchRecapsRequest{
		Query: q,
		Limit: safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, 0, err
	}

	results := make([]RecapHit, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		results[i] = RecapHit{
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
