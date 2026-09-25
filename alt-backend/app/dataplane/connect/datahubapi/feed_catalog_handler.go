package datahubapi

import (
	"context"
	"errors"
	"fmt"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/shared/driver/alt_db"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// §2.H Feeds
// ---------------------------------------------------------------------------

// maxFeedRegistrationBatch bounds one poll's worth of items. The whole batch
// is one transaction, so an unbounded request is an unbounded lock hold.
const maxFeedRegistrationBatch = 2000

func (h *Handler) RegisterFeeds(ctx context.Context, req *connect.Request[datahubv1.RegisterFeedsRequest]) (*connect.Response[datahubv1.RegisterFeedsResponse], error) {
	items := req.Msg.GetFeeds()
	if len(items) == 0 {
		return connect.NewResponse(&datahubv1.RegisterFeedsResponse{
			Results: []*datahubv1.FeedRegistrationResult{},
		}), nil
	}
	if len(items) > maxFeedRegistrationBatch {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feeds exceeds the %d entry limit", maxFeedRegistrationBatch))
	}

	registrations := make([]domain.FeedRegistration, 0, len(items))
	for _, f := range items {
		if f.GetWebsiteUrl() == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("website_url is required for every feed"))
		}
		registrations = append(registrations, domain.FeedRegistration{
			Title:       f.GetTitle(),
			Description: f.GetDescription(),
			WebsiteURL:  f.GetWebsiteUrl(),
			PubDate:     f.GetPubDate().AsTime(),
			CreatedAt:   f.GetCreatedAt().AsTime(),
			UpdatedAt:   f.GetUpdatedAt().AsTime(),
			FeedLinkID:  f.FeedLinkId,
			OgImageURL:  f.OgImageUrl,
		})
	}

	results, err := h.feed.Register(ctx, registrations)
	if err != nil {
		h.logger.ErrorContext(ctx, "RegisterFeeds failed", "error", err, "feed_count", len(items))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to register feeds"))
	}

	out := make([]*datahubv1.FeedRegistrationResult, 0, len(results))
	for _, r := range results {
		out = append(out, &datahubv1.FeedRegistrationResult{FeedId: r.FeedID, Created: r.Created})
	}
	return connect.NewResponse(&datahubv1.RegisterFeedsResponse{Results: out}), nil
}

func (h *Handler) ListFeedsCursor(ctx context.Context, req *connect.Request[datahubv1.ListFeedsCursorRequest]) (*connect.Response[datahubv1.ListFeedsCursorResponse], error) {
	scope, err := feedScopeFromProto(req.Msg.GetScope())
	if err != nil {
		return nil, err
	}

	userID, err := requiredUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	excludes, err := parseUUIDs(req.Msg.GetExcludeFeedLinkIds(), "exclude_feed_link_ids")
	if err != nil {
		return nil, err
	}

	rows, err := h.feed.ListCursor(ctx, scope, userID,
		timePtrOrNil(req.Msg.GetCursor()), clampFeedLimit(int(req.Msg.GetLimit())), excludes)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedsCursor failed", "error", err, "scope", req.Msg.GetScope().String())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feeds"))
	}

	return connect.NewResponse(&datahubv1.ListFeedsCursorResponse{Feeds: feedRowsToProto(rows)}), nil
}

func (h *Handler) ListFeedsPage(ctx context.Context, req *connect.Request[datahubv1.ListFeedsPageRequest]) (*connect.Response[datahubv1.ListFeedsPageResponse], error) {
	if req.Msg.GetPage() < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("page must not be negative"))
	}

	// user_id is required only for the unread page. Requiring it for both
	// would break the unscoped list; accepting an absent one for the unread
	// page would silently answer with every user's feeds.
	var userID uuid.UUID
	if req.Msg.GetUnreadOnly() {
		parsed, err := requiredUUID(req.Msg.GetUserId(), "user_id")
		if err != nil {
			return nil, err
		}
		userID = parsed
	}

	rows, err := h.feed.ListPage(ctx, int(req.Msg.GetPage()), req.Msg.GetUnreadOnly(), userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedsPage failed", "error", err, "page", req.Msg.GetPage())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feeds page"))
	}

	return connect.NewResponse(&datahubv1.ListFeedsPageResponse{Feeds: feedRowsToProto(rows)}), nil
}

func (h *Handler) ListFeedsLimit(ctx context.Context, req *connect.Request[datahubv1.ListFeedsLimitRequest]) (*connect.Response[datahubv1.ListFeedsLimitResponse], error) {
	rows, err := h.feed.ListLimit(ctx, int(req.Msg.GetLimit()))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedsLimit failed", "error", err, "limit", req.Msg.GetLimit())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feeds"))
	}
	return connect.NewResponse(&datahubv1.ListFeedsLimitResponse{Feeds: feedRowsToProto(rows)}), nil
}

func (h *Handler) GetSingleFeed(ctx context.Context, _ *connect.Request[datahubv1.GetSingleFeedRequest]) (*connect.Response[datahubv1.GetSingleFeedResponse], error) {
	row, err := h.feed.GetSingle(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetSingleFeed failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get single feed"))
	}
	return connect.NewResponse(&datahubv1.GetSingleFeedResponse{Feed: feedRowToProto(row)}), nil
}

func (h *Handler) ListFeedsByFeedLinkID(ctx context.Context, req *connect.Request[datahubv1.ListFeedsByFeedLinkIDRequest]) (*connect.Response[datahubv1.ListFeedsByFeedLinkIDResponse], error) {
	feedLinkID, err := requiredUUID(req.Msg.GetFeedLinkId(), "feed_link_id")
	if err != nil {
		return nil, err
	}

	rows, err := h.feed.ListByFeedLinkID(ctx, feedLinkID)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedsByFeedLinkID failed", "error", err, "feed_link_id", req.Msg.GetFeedLinkId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feeds by feed link"))
	}
	return connect.NewResponse(&datahubv1.ListFeedsByFeedLinkIDResponse{Feeds: feedRowsToProto(rows)}), nil
}

func (h *Handler) GetFeedSummary(ctx context.Context, req *connect.Request[datahubv1.GetFeedSummaryRequest]) (*connect.Response[datahubv1.GetFeedSummaryResponse], error) {
	if req.Msg.GetFeedUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	userID, err := optionalUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	summary, err := h.feed.GetSummary(ctx, req.Msg.GetFeedUrl(), userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetFeedSummary failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get feed summary"))
	}
	return connect.NewResponse(&datahubv1.GetFeedSummaryResponse{Summary: feedSummaryToProto(summary)}), nil
}

func (h *Handler) SearchFeedsByTitle(ctx context.Context, req *connect.Request[datahubv1.SearchFeedsByTitleRequest]) (*connect.Response[datahubv1.SearchFeedsByTitleResponse], error) {
	if _, err := requiredUUID(req.Msg.GetUserId(), "user_id"); err != nil {
		return nil, err
	}

	rows, err := h.feed.SearchByTitle(ctx, req.Msg.GetQuery(), req.Msg.GetUserId())
	if err != nil {
		h.logger.ErrorContext(ctx, "SearchFeedsByTitle failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to search feeds"))
	}
	return connect.NewResponse(&datahubv1.SearchFeedsByTitleResponse{Feeds: feedRowsToProto(rows)}), nil
}

func (h *Handler) GetRandomFeed(ctx context.Context, _ *connect.Request[datahubv1.GetRandomFeedRequest]) (*connect.Response[datahubv1.GetRandomFeedResponse], error) {
	feed, err := h.feed.GetRandom(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetRandomFeed failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get random feed"))
	}
	if feed == nil {
		// Unset rather than NotFound: "nothing is tagged yet" is a state the
		// Tag Trail entry point renders, not a failure it reports.
		return connect.NewResponse(&datahubv1.GetRandomFeedResponse{}), nil
	}

	return connect.NewResponse(&datahubv1.GetRandomFeedResponse{
		Feed: &datahubv1.Feed{
			Id:          feed.ID.String(),
			Title:       feed.Title,
			Description: feed.Description,
			WebsiteUrl:  feed.WebsiteURL,
		},
	}), nil
}

func (h *Handler) GetFeedURLsByArticleIDs(ctx context.Context, req *connect.Request[datahubv1.GetFeedURLsByArticleIDsRequest]) (*connect.Response[datahubv1.GetFeedURLsByArticleIDsResponse], error) {
	ids := req.Msg.GetArticleIds()
	if len(ids) == 0 {
		return connect.NewResponse(&datahubv1.GetFeedURLsByArticleIDsResponse{
			Pairs: []*datahubv1.FeedAndArticle{},
		}), nil
	}
	if len(ids) > maxLimit {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_ids exceeds the %d entry limit", maxLimit))
	}

	pairs, err := h.feed.GetFeedURLsByArticleIDs(ctx, ids)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetFeedURLsByArticleIDs failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get feed urls"))
	}

	out := make([]*datahubv1.FeedAndArticle, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, &datahubv1.FeedAndArticle{
			FeedId:       p.FeedID,
			ArticleId:    p.ArticleID,
			Url:          p.URL,
			FeedTitle:    p.FeedTitle,
			ArticleTitle: p.ArticleTitle,
		})
	}
	return connect.NewResponse(&datahubv1.GetFeedURLsByArticleIDsResponse{Pairs: out}), nil
}

func (h *Handler) BatchGetFeedTitlesByIDs(ctx context.Context, req *connect.Request[datahubv1.BatchGetFeedTitlesByIDsRequest]) (*connect.Response[datahubv1.BatchGetFeedTitlesByIDsResponse], error) {
	raw := req.Msg.GetFeedIds()
	if len(raw) == 0 {
		return connect.NewResponse(&datahubv1.BatchGetFeedTitlesByIDsResponse{
			Titles: map[string]string{},
		}), nil
	}
	if len(raw) > maxLimit {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_ids exceeds the %d entry limit", maxLimit))
	}

	ids, err := parseUUIDs(raw, "feed_ids")
	if err != nil {
		return nil, err
	}

	titles, err := h.feed.BatchGetTitlesByIDs(ctx, ids)
	if err != nil {
		h.logger.ErrorContext(ctx, "BatchGetFeedTitlesByIDs failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to batch get feed titles"))
	}

	out := make(map[string]string, len(titles))
	for id, title := range titles {
		out[id.String()] = title
	}
	return connect.NewResponse(&datahubv1.BatchGetFeedTitlesByIDsResponse{Titles: out}), nil
}

func (h *Handler) GetInoreaderSummariesByURLs(ctx context.Context, req *connect.Request[datahubv1.GetInoreaderSummariesByURLsRequest]) (*connect.Response[datahubv1.GetInoreaderSummariesByURLsResponse], error) {
	urls := req.Msg.GetUrls()
	if len(urls) == 0 {
		return connect.NewResponse(&datahubv1.GetInoreaderSummariesByURLsResponse{
			Summaries: []*datahubv1.InoreaderSummary{},
		}), nil
	}
	if len(urls) > maxLimit {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("urls exceeds the %d entry limit", maxLimit))
	}

	summaries, err := h.feed.GetInoreaderSummariesByURLs(ctx, urls)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetInoreaderSummariesByURLs failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get inoreader summaries"))
	}

	out := make([]*datahubv1.InoreaderSummary, 0, len(summaries))
	for _, s := range summaries {
		if s == nil {
			continue
		}
		out = append(out, &datahubv1.InoreaderSummary{
			ArticleUrl:  s.ArticleURL,
			Title:       s.Title,
			Author:      s.Author,
			Content:     s.Content,
			ContentType: s.ContentType,
			PublishedAt: timestamppb.New(s.PublishedAt),
			FetchedAt:   timestamppb.New(s.FetchedAt),
			InoreaderId: s.InoreaderID,
		})
	}
	return connect.NewResponse(&datahubv1.GetInoreaderSummariesByURLsResponse{Summaries: out}), nil
}

func (h *Handler) GetFeedID(ctx context.Context, req *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
	if h.getFeedID == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.FeedUrl == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	feedID, err := h.getFeedID.GetFeedID(ctx, req.Msg.FeedUrl)
	if err != nil {
		// pre-processor reads NotFound as "this feed is not registered": it
		// skips every article of the batch and treats the URL as unknown on
		// the existence check. A pool exhaustion or a transient DB blip
		// answered with the same code would silently drop a whole batch of
		// ingested articles, so only the driver's absence sentinel earns it.
		if errors.Is(err, alt_db.ErrFeedNotFoundByURL) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("feed not found"))
		}
		h.logger.Error("GetFeedID failed", "feed_url", req.Msg.FeedUrl, "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get feed ID"))
	}

	return connect.NewResponse(&datahubv1.GetFeedIDResponse{
		FeedId: feedID,
	}), nil
}

func (h *Handler) ListFeedURLs(ctx context.Context, req *connect.Request[datahubv1.ListFeedURLsRequest]) (*connect.Response[datahubv1.ListFeedURLsResponse], error) {
	if h.listFeedURLs == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	limit := clampLimit(int(req.Msg.Limit))
	feeds, nextCursor, hasMore, err := h.listFeedURLs.ListFeedURLs(ctx, req.Msg.Cursor, limit)
	if err != nil {
		h.logger.Error("ListFeedURLs failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feed URLs"))
	}

	protoFeeds := make([]*datahubv1.FeedURL, len(feeds))
	for i, f := range feeds {
		protoFeeds[i] = &datahubv1.FeedURL{
			FeedId: f.FeedID,
			Url:    f.URL,
		}
	}

	return connect.NewResponse(&datahubv1.ListFeedURLsResponse{
		Feeds:      protoFeeds,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}), nil
}

// ── Backfill operations (pre-processor split-DB) ──

func (h *Handler) GetEmptyFeedID(ctx context.Context, req *connect.Request[datahubv1.GetEmptyFeedIDRequest]) (*connect.Response[datahubv1.GetEmptyFeedIDResponse], error) {
	if h.getEmptyFeedID == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.FeedUrl == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	feedID, err := h.getEmptyFeedID.GetEmptyFeedID(ctx, req.Msg.FeedUrl)
	if err != nil {
		h.logger.Error("GetEmptyFeedID failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get empty feed ID"))
	}

	return connect.NewResponse(&datahubv1.GetEmptyFeedIDResponse{
		FeedId: feedID,
	}), nil
}
