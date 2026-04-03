package altdb

import (
	"context"
	"fmt"
	"log/slog"

	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/gen/proto/alt/articles/v2/articlesv2connect"

	"rag-orchestrator/internal/domain"

	"connectrpc.com/connect"
)

// ConnectTagCloudClient implements domain.TagCloudClient using Connect-RPC.
type ConnectTagCloudClient struct {
	client articlesv2connect.ArticleServiceClient
	logger *slog.Logger
}

// NewConnectTagCloudClient creates a tag cloud client using the given Connect-RPC client.
func NewConnectTagCloudClient(client articlesv2connect.ArticleServiceClient, logger *slog.Logger) *ConnectTagCloudClient {
	return &ConnectTagCloudClient{client: client, logger: logger}
}

func (c *ConnectTagCloudClient) FetchTagCloud(ctx context.Context, limit int) ([]domain.TagCloudEntry, error) {
	req := connect.NewRequest(&articlesv2.FetchTagCloudRequest{
		Limit: int32(limit),
	})

	resp, err := c.client.FetchTagCloud(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("FetchTagCloud RPC failed: %w", err)
	}

	tags := make([]domain.TagCloudEntry, 0, len(resp.Msg.Tags))
	for _, t := range resp.Msg.Tags {
		tags = append(tags, domain.TagCloudEntry{
			TagName:      t.TagName,
			ArticleCount: t.ArticleCount,
		})
	}

	c.logger.Debug("tag_cloud_fetched", slog.Int("tags", len(tags)))
	return tags, nil
}
