package altdb

import (
	"context"
	"fmt"
	"log/slog"

	recapv2 "alt/gen/proto/alt/recap/v2"
	"alt/gen/proto/alt/recap/v2/recapv2connect"

	"rag-orchestrator/internal/domain"

	"connectrpc.com/connect"
)

// ConnectRecapSearchClient implements domain.RecapSearchClient using Connect-RPC.
type ConnectRecapSearchClient struct {
	client recapv2connect.RecapServiceClient
	logger *slog.Logger
}

// NewConnectRecapSearchClient creates a recap search client.
func NewConnectRecapSearchClient(client recapv2connect.RecapServiceClient, logger *slog.Logger) *ConnectRecapSearchClient {
	return &ConnectRecapSearchClient{client: client, logger: logger}
}

func (c *ConnectRecapSearchClient) SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]domain.RecapSearchResult, error) {
	req := connect.NewRequest(&recapv2.SearchRecapsByTagRequest{
		TagName: tagName,
		Limit:   int32(limit),
	})

	resp, err := c.client.SearchRecapsByTag(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("SearchRecapsByTag RPC failed: %w", err)
	}

	results := make([]domain.RecapSearchResult, 0, len(resp.Msg.Results))
	for _, r := range resp.Msg.Results {
		results = append(results, domain.RecapSearchResult{
			Genre:    r.GetGenre(),
			Summary:  r.GetSummary(),
			TopTerms: r.GetTopTerms(),
		})
	}

	c.logger.Debug("recaps_searched", slog.String("tag", tagName), slog.Int("results", len(results)))
	return results, nil
}
