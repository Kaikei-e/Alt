package altdb

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"rag-orchestrator/internal/domain"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// DataHubArticleClient implements domain.ArticleClient on top of
// services.datahub.v1.DataHubService/ListRecentArticles.
//
// It replaces the REST client that called
// GET http://alt-backend:9102/v1/internal/articles/recent. That route did not
// move — it was absorbed (ADR-000954 D6): alt-backend's :9102 is an
// admin-only operator listener after the 3-binary split, and alt-data-hub is
// the only process that can reach alt_db. Authentication is the client
// certificate presented by the transport; there is no application-layer
// credential.
type DataHubArticleClient struct {
	client datahubv1connect.DataHubServiceClient
	logger *slog.Logger
}

// NewDataHubArticleClient creates an article client over DataHubService.
func NewDataHubArticleClient(client datahubv1connect.DataHubServiceClient, logger *slog.Logger) *DataHubArticleClient {
	return &DataHubArticleClient{client: client, logger: logger}
}

// GetRecentArticles fetches articles published within withinHours.
//
// Both request fields are set explicitly, and that is load-bearing. The proto
// declares them `optional` so that "unset" and "0" stay distinguishable: an
// unset limit means the server default of 100, while limit = 0 means "time
// window only, no count cap" — the mode the morning-letter usecase runs in.
// Letting protojson elide the zero would silently truncate the result to 100
// articles. Sending both fields is what preserves the REST route's semantics.
func (c *DataHubArticleClient) GetRecentArticles(ctx context.Context, withinHours int, limit int) ([]domain.ArticleMetadata, error) {
	withinHours32 := int32(min(withinHours, math.MaxInt32)) //nolint:gosec // clamped to MaxInt32
	limit32 := int32(min(limit, math.MaxInt32))             //nolint:gosec // clamped to MaxInt32

	req := connect.NewRequest(&datahubv1.ListRecentArticlesRequest{
		WithinHours: &withinHours32,
		Limit:       &limit32,
	})

	c.logger.Info("fetching recent articles from alt-data-hub",
		slog.Int("within_hours", withinHours),
		slog.Int("limit", limit))

	resp, err := c.client.ListRecentArticles(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ListRecentArticles RPC failed: %w", err)
	}

	c.logger.Info("successfully fetched recent articles",
		slog.Int("count", int(resp.Msg.GetCount())),
		slog.String("since", resp.Msg.GetSince()),
		slog.String("until", resp.Msg.GetUntil()))

	items := resp.Msg.GetArticles()
	articles := make([]domain.ArticleMetadata, 0, len(items))
	for _, item := range items {
		articleID, err := uuid.Parse(item.GetId())
		if err != nil {
			c.logger.Warn("invalid article ID, skipping",
				slog.String("id", item.GetId()),
				slog.String("error", err.Error()))
			continue
		}

		feedID, err := uuid.Parse(item.GetFeedId())
		if err != nil {
			// FeedID may be absent; the zero UUID is the documented stand-in.
			feedID = uuid.Nil
		}

		// published_at is a business fact (it drives temporal boost scoring
		// downstream); a parse failure must not be papered over with the
		// wall-clock time. Keep the zero value so downstream temporal
		// scoring can detect and deprioritize it instead of scoring it as
		// "just published".
		publishedAt, err := time.Parse(time.RFC3339, item.GetPublishedAt())
		if err != nil {
			c.logger.Warn("invalid published_at, keeping zero value",
				slog.String("published_at", item.GetPublishedAt()))
			publishedAt = time.Time{}
		}

		articles = append(articles, domain.ArticleMetadata{
			ID:          articleID,
			Title:       item.GetTitle(),
			URL:         item.GetUrl(),
			PublishedAt: publishedAt,
			FeedID:      feedID,
			Tags:        item.GetTags(),
		})
	}

	return articles, nil
}
