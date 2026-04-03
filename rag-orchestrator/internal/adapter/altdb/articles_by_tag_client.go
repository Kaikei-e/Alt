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

// ConnectArticlesByTagClient implements domain.ArticlesByTagClient using Connect-RPC.
type ConnectArticlesByTagClient struct {
	client articlesv2connect.ArticleServiceClient
	logger *slog.Logger
}

// NewConnectArticlesByTagClient creates an articles-by-tag client.
func NewConnectArticlesByTagClient(client articlesv2connect.ArticleServiceClient, logger *slog.Logger) *ConnectArticlesByTagClient {
	return &ConnectArticlesByTagClient{client: client, logger: logger}
}

func (c *ConnectArticlesByTagClient) FetchArticlesByTag(ctx context.Context, tagName string, limit int) ([]domain.TagArticle, error) {
	req := connect.NewRequest(&articlesv2.FetchArticlesByTagRequest{
		TagName: &tagName,
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
			URL:   a.GetLink(),
		})
	}

	c.logger.Debug("articles_by_tag_fetched",
		slog.String("tag", tagName),
		slog.Int("count", len(articles)))
	return articles, nil
}
