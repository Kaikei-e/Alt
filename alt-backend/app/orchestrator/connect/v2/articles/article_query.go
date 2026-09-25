package articles

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"alt/connect/errorhandler"
	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/utils/perf"
)

// FetchArticlesCursor fetches articles with cursor-based pagination.
// Replaces GET /v1/articles/fetch/cursor
func (h *Handler) FetchArticlesCursor(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticlesCursorRequest],
) (*connect.Response[articlesv2.FetchArticlesCursorResponse], error) {
	if _, err := requireUser(ctx); err != nil {
		return nil, err
	}

	limit := clampPageLimit(req.Msg.Limit, 20, maxArticlesPageSize)

	var cursor *time.Time
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339, *req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid cursor format, expected RFC3339: %w", err))
		}
		cursor = &parsed
	}

	timer := perf.NewFeedReadTimer("FetchArticlesCursor")

	stopUsecase := timer.StartPhase(ctx, "usecase")
	articles, err := h.deps.FetchArticlesCursor.Execute(ctx, cursor, limit+1)
	stopUsecase()
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchArticlesCursor")
	}

	hasMore := len(articles) > limit
	if hasMore {
		articles = articles[:limit]
	}

	tagCount := 0
	for _, a := range articles {
		tagCount += len(a.Tags)
	}

	stopMarshal := timer.StartPhase(ctx, "marshal")
	protoArticles := convertArticlesToProto(articles)

	// Derive next cursor, with sub-second precision: it comes back as the
	// right-hand side of a strict `created_at < $1`, and created_at is
	// microsecond precision.
	var nextCursor *string
	if len(articles) > 0 {
		nextCursor = deriveRFC3339NanoCursor(articles[len(articles)-1].PublishedAt, hasMore)
	}

	respMsg := &articlesv2.FetchArticlesCursorResponse{
		Data:       protoArticles,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}
	resp := connect.NewResponse(respMsg)
	stopMarshal()

	timer.SetRowCount(len(articles))
	timer.SetPayloadBytes(int64(proto.Size(respMsg)))
	timer.SetTagCount(tagCount)
	timer.Log(ctx)
	return resp, nil
}

// FetchRandomFeed fetches a random feed for Tag Trail.
// Replaces GET /v1/rss-feed-link/random
// ADR-173: Includes tags for the feed's latest article (generated on-the-fly if not in DB)
func (h *Handler) FetchRandomFeed(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchRandomFeedRequest],
) (*connect.Response[articlesv2.FetchRandomFeedResponse], error) {
	if _, err := requireUser(ctx); err != nil {
		return nil, err
	}

	feed, err := h.deps.FetchRandomSubscription.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchRandomFeed")
	}

	var protoTags []*articlesv2.ArticleTagItem
	var latestArticleID string

	if h.deps.FetchLatestArticle != nil {
		latestArticle, err := h.deps.FetchLatestArticle.Execute(ctx, feed.ID)
		if err != nil {
			h.logger.WarnContext(ctx, "failed to fetch latest article for feed", "feedID", feed.ID, "error", err)
		} else if latestArticle != nil {
			latestArticleID = latestArticle.ID
			h.logger.InfoContext(ctx, "found latest article for feed", "feedID", feed.ID, "articleID", latestArticle.ID)

			tags, err := h.deps.FetchArticleTags.Execute(ctx, latestArticle.ID)
			if err != nil {
				h.logger.WarnContext(ctx, "failed to fetch/generate tags for article", "articleID", latestArticle.ID, "error", err)
			} else {
				protoTags = convertTagsToProto(tags)
				h.logger.InfoContext(ctx, "fetched tags for feed's latest article",
					"feedID", feed.ID,
					"articleID", latestArticle.ID,
					"tagCount", len(protoTags))
			}
		} else {
			h.logger.InfoContext(ctx, "no articles found for feed", "feedID", feed.ID)
		}
	}

	return connect.NewResponse(&articlesv2.FetchRandomFeedResponse{
		Id:              feed.ID.String(),
		Url:             feed.WebsiteURL,
		Title:           feed.Title,
		Description:     feed.Description,
		Tags:            protoTags,
		LatestArticleId: latestArticleID,
	}), nil
}
