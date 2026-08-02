package altdb

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"rag-orchestrator/internal/domain"

	"connectrpc.com/connect"
)

// DataHubTagCloudClient implements domain.TagCloudClient using
// services.datahub.v1.DataHubService/FetchTagCloud.
//
// Origin: services.backend.v1.BackendInternalService/FetchTagCloud. The
// request and response messages are wire-identical to the legacy pair
// (ADR-000954 D7); only the package on the URL path changed, along with the
// process that answers it and the mutual TLS in front of it.
type DataHubTagCloudClient struct {
	client datahubv1connect.DataHubServiceClient
	logger *slog.Logger
}

// NewDataHubTagCloudClient creates a tag cloud client using DataHubService.
func NewDataHubTagCloudClient(client datahubv1connect.DataHubServiceClient, logger *slog.Logger) *DataHubTagCloudClient {
	return &DataHubTagCloudClient{client: client, logger: logger}
}

func (c *DataHubTagCloudClient) FetchTagCloud(ctx context.Context, limit int) ([]domain.TagCloudEntry, error) {
	req := connect.NewRequest(&datahubv1.FetchTagCloudRequest{
		Limit: int32(min(limit, math.MaxInt32)), //nolint:gosec // value clamped to MaxInt32
	})

	resp, err := c.client.FetchTagCloud(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("FetchTagCloud RPC failed: %w", err)
	}

	tags := make([]domain.TagCloudEntry, 0, len(resp.Msg.GetTags()))
	for _, t := range resp.Msg.GetTags() {
		tags = append(tags, domain.TagCloudEntry{
			TagName:      t.GetTagName(),
			ArticleCount: t.GetArticleCount(),
		})
	}

	c.logger.Debug("tag_cloud_fetched", slog.Int("tags", len(tags)))
	return tags, nil
}
