package altdb

import (
	"context"
	"fmt"
	"log/slog"

	backendv1 "alt/gen/proto/services/backend/v1"
	"alt/gen/proto/services/backend/v1/backendv1connect"

	"rag-orchestrator/internal/domain"

	"connectrpc.com/connect"
)

// InternalTagCloudClient implements domain.TagCloudClient using BackendInternalService Connect-RPC.
type InternalTagCloudClient struct {
	client backendv1connect.BackendInternalServiceClient
	logger *slog.Logger
}

// NewInternalTagCloudClient creates a tag cloud client using the BackendInternalService.
func NewInternalTagCloudClient(client backendv1connect.BackendInternalServiceClient, logger *slog.Logger) *InternalTagCloudClient {
	return &InternalTagCloudClient{client: client, logger: logger}
}

func (c *InternalTagCloudClient) FetchTagCloud(ctx context.Context, limit int) ([]domain.TagCloudEntry, error) {
	req := connect.NewRequest(&backendv1.BackendInternalServiceFetchTagCloudRequest{
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
