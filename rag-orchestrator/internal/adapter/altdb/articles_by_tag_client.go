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

// DataHubArticlesByTagClient implements domain.ArticlesByTagClient using
// services.datahub.v1.DataHubService/FetchArticlesByTag.
//
// Origin: services.backend.v1.BackendInternalService/FetchArticlesByTag,
// wire-identical messages (ADR-000954 D7).
type DataHubArticlesByTagClient struct {
	client datahubv1connect.DataHubServiceClient
	logger *slog.Logger
}

// NewDataHubArticlesByTagClient creates an articles-by-tag client using
// DataHubService.
func NewDataHubArticlesByTagClient(client datahubv1connect.DataHubServiceClient, logger *slog.Logger) *DataHubArticlesByTagClient {
	return &DataHubArticlesByTagClient{client: client, logger: logger}
}

func (c *DataHubArticlesByTagClient) FetchArticlesByTag(ctx context.Context, tagName string, limit int) ([]domain.TagArticle, error) {
	req := connect.NewRequest(&datahubv1.FetchArticlesByTagRequest{
		TagName: tagName,
		Limit:   int32(min(limit, math.MaxInt32)), //nolint:gosec // value clamped to MaxInt32
	})

	resp, err := c.client.FetchArticlesByTag(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("FetchArticlesByTag RPC failed: %w", err)
	}

	articles := make([]domain.TagArticle, 0, len(resp.Msg.GetArticles()))
	for _, a := range resp.Msg.GetArticles() {
		articles = append(articles, domain.TagArticle{
			ID:    a.GetId(),
			Title: a.GetTitle(),
			URL:   a.GetUrl(),
		})
	}

	c.logger.Debug("articles_by_tag_fetched",
		slog.String("tag", tagName),
		slog.Int("count", len(articles)))
	return articles, nil
}
