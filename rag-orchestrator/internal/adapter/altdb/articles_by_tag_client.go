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

// InternalArticlesByTagClient implements domain.ArticlesByTagClient using BackendInternalService.
type InternalArticlesByTagClient struct {
	client backendv1connect.BackendInternalServiceClient
	logger *slog.Logger
}

// NewInternalArticlesByTagClient creates an articles-by-tag client using BackendInternalService.
func NewInternalArticlesByTagClient(client backendv1connect.BackendInternalServiceClient, logger *slog.Logger) *InternalArticlesByTagClient {
	return &InternalArticlesByTagClient{client: client, logger: logger}
}

func (c *InternalArticlesByTagClient) FetchArticlesByTag(ctx context.Context, tagName string, limit int) ([]domain.TagArticle, error) {
	req := connect.NewRequest(&backendv1.BackendInternalServiceFetchArticlesByTagRequest{
		TagName: tagName,
		Limit:   int32(limit),
	})

	resp, err := c.client.FetchArticlesByTag(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("FetchArticlesByTag RPC failed: %w", err)
	}

	articles := make([]domain.TagArticle, 0, len(resp.Msg.Articles))
	for _, a := range resp.Msg.Articles {
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
