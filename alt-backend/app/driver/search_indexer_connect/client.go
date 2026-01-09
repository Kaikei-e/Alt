// Package search_indexer_connect provides Connect-RPC client for search-indexer service.
package search_indexer_connect

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	"alt/domain"
	searchv2 "alt/gen/proto/clients/search/v2"
	"alt/gen/proto/clients/search/v2/searchv2connect"
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
