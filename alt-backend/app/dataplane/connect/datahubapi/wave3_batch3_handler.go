package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// maxFeedLimit bounds the cursor and limit walks over feeds. Same ceiling as
// the article walks; the value is the provider's, because a caller that asks
// for more than the provider will plan for should be told, not quietly served
// a shorter page it will mistake for the end of the list.
const maxFeedLimit = 500

// WithWave3Batch3Capabilities wires the feed and feed-link capabilities
// ADR-000954 Wave 3 batch 3 moved out of alt-backend and alt-harvester
// (capability catalog §2.F / §2.G / §2.H).
//
// Nil panics, for the same reason the earlier batches give: after this batch
// these procedures are the only route to feed_links, feed_link_availability
// and feeds. A data hub started with a nil feed port would answer
// ListFeedsCursor with Unimplemented — which is also what a retired procedure
// answers — and every user would see an empty feed list while health checks
// stayed green (CLAUDE.md rule 8, ADR-000928).
func WithWave3Batch3Capabilities(
	feedLink datahub_capability_port.FeedLinkPort,
	feedLinkAvailability datahub_capability_port.FeedLinkAvailabilityPort,
	feed datahub_capability_port.FeedPort,
) HandlerOption {
	switch {
	case feedLink == nil:
		panic("datahubapi: FeedLinkPort is required — feed registration and the collector's work list have no other route to feed_links")
	case feedLinkAvailability == nil:
		panic("datahubapi: FeedLinkAvailabilityPort is required — without it a dead feed is polled forever")
	case feed == nil:
		panic("datahubapi: FeedPort is required — every feed-serving surface reads through it")
	}

	return func(h *Handler) {
		h.feedLink = feedLink
		h.feedLinkAvailability = feedLinkAvailability
		h.feed = feed
	}
}

// ---------------------------------------------------------------------------
// §2.F Feed links
// ---------------------------------------------------------------------------

func (h *Handler) RegisterFeedLink(ctx context.Context, req *connect.Request[datahubv1.RegisterFeedLinkRequest]) (*connect.Response[datahubv1.RegisterFeedLinkResponse], error) {
	if req.Msg.GetUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url is required"))
	}

	alreadyExisted, err := h.feedLink.Register(ctx, req.Msg.GetUrl())
	if err != nil {
		h.logger.ErrorContext(ctx, "RegisterFeedLink failed", "error", err, "url", req.Msg.GetUrl())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to register feed link"))
	}

	return connect.NewResponse(&datahubv1.RegisterFeedLinkResponse{
		AlreadyExisted: alreadyExisted,
	}), nil
}

func (h *Handler) BulkRegisterFeedLinks(ctx context.Context, req *connect.Request[datahubv1.BulkRegisterFeedLinksRequest]) (*connect.Response[datahubv1.BulkRegisterFeedLinksResponse], error) {
	urls := req.Msg.GetUrls()
	if len(urls) == 0 {
		return connect.NewResponse(&datahubv1.BulkRegisterFeedLinksResponse{FailedUrls: []string{}}), nil
	}
	if len(urls) > maxBulkFeedLinks {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("urls exceeds the %d entry limit", maxBulkFeedLinks))
	}

	registered, skipped, failed, err := h.feedLink.BulkRegister(ctx, urls)
	if err != nil {
		h.logger.ErrorContext(ctx, "BulkRegisterFeedLinks failed", "error", err, "url_count", len(urls))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to bulk register feed links"))
	}

	return connect.NewResponse(&datahubv1.BulkRegisterFeedLinksResponse{
		Registered: safeconv.Int32(registered),
		Skipped:    safeconv.Int32(skipped),
		FailedUrls: failed,
	}), nil
}

// maxBulkFeedLinks caps one OPML import. The bound is here rather than in the
// gateway because only the delivery layer can refuse the request outright; a
// gateway that truncated would import a prefix and report success for the
// whole file.
const maxBulkFeedLinks = 5000

func (h *Handler) ListFeedLinks(ctx context.Context, _ *connect.Request[datahubv1.ListFeedLinksRequest]) (*connect.Response[datahubv1.ListFeedLinksResponse], error) {
	links, err := h.feedLink.List(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedLinks failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feed links"))
	}

	out := make([]*datahubv1.FeedLink, 0, len(links))
	for _, l := range links {
		if l == nil {
			continue
		}
		out = append(out, &datahubv1.FeedLink{Id: l.ID.String(), Url: l.URL})
	}
	return connect.NewResponse(&datahubv1.ListFeedLinksResponse{FeedLinks: out}), nil
}

func (h *Handler) ListFeedLinksWithHealth(ctx context.Context, _ *connect.Request[datahubv1.ListFeedLinksWithHealthRequest]) (*connect.Response[datahubv1.ListFeedLinksWithHealthResponse], error) {
	links, err := h.feedLink.ListWithHealth(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedLinksWithHealth failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feed links with health"))
	}

	out := make([]*datahubv1.FeedLinkWithHealth, 0, len(links))
	for _, l := range links {
		if l == nil {
			continue
		}
		out = append(out, &datahubv1.FeedLinkWithHealth{
			FeedLink: &datahubv1.FeedLink{Id: l.ID.String(), Url: l.URL},
			// Left unset for a link that has never been polled. The admin
			// screen renders "unknown" for the absence and "healthy" for a
			// zero-failure row, and those are different facts.
			Availability: feedLinkAvailabilityToProto(l.Availability),
		})
	}
	return connect.NewResponse(&datahubv1.ListFeedLinksWithHealthResponse{FeedLinks: out}), nil
}

func (h *Handler) DeleteFeedLink(ctx context.Context, req *connect.Request[datahubv1.DeleteFeedLinkRequest]) (*connect.Response[datahubv1.DeleteFeedLinkResponse], error) {
	id, err := requiredUUID(req.Msg.GetId(), "id")
	if err != nil {
		return nil, err
	}

	if err := h.feedLink.Delete(ctx, id); err != nil {
		h.logger.ErrorContext(ctx, "DeleteFeedLink failed", "error", err, "id", req.Msg.GetId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete feed link"))
	}
	return connect.NewResponse(&datahubv1.DeleteFeedLinkResponse{}), nil
}

func (h *Handler) ResolveFeedLinkIDByURL(ctx context.Context, req *connect.Request[datahubv1.ResolveFeedLinkIDByURLRequest]) (*connect.Response[datahubv1.ResolveFeedLinkIDByURLResponse], error) {
	if req.Msg.GetFeedUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	id, err := h.feedLink.ResolveIDByURL(ctx, req.Msg.GetFeedUrl())
	if err != nil {
		h.logger.ErrorContext(ctx, "ResolveFeedLinkIDByURL failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve feed link id"))
	}

	return connect.NewResponse(&datahubv1.ResolveFeedLinkIDByURLResponse{FeedLinkId: id}), nil
}

func (h *Handler) ListFeedLinkDomains(ctx context.Context, _ *connect.Request[datahubv1.ListFeedLinkDomainsRequest]) (*connect.Response[datahubv1.ListFeedLinkDomainsResponse], error) {
	domains, err := h.feedLink.ListDomains(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedLinkDomains failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feed link domains"))
	}

	out := make([]*datahubv1.FeedLinkDomain, 0, len(domains))
	for _, d := range domains {
		out = append(out, &datahubv1.FeedLinkDomain{Domain: d.Domain, Scheme: d.Scheme})
	}
	return connect.NewResponse(&datahubv1.ListFeedLinkDomainsResponse{Domains: out}), nil
}

func (h *Handler) ListRSSFeedURLs(ctx context.Context, _ *connect.Request[datahubv1.ListRSSFeedURLsRequest]) (*connect.Response[datahubv1.ListRSSFeedURLsResponse], error) {
	links, err := h.feedLink.ListPollable(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListRSSFeedURLs failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list rss feed urls"))
	}

	out := make([]*datahubv1.FeedLink, 0, len(links))
	for _, l := range links {
		out = append(out, &datahubv1.FeedLink{Id: l.ID.String(), Url: l.URL})
	}
	return connect.NewResponse(&datahubv1.ListRSSFeedURLsResponse{FeedLinks: out}), nil
}

func (h *Handler) ListFeedLinksForExport(ctx context.Context, _ *connect.Request[datahubv1.ListFeedLinksForExportRequest]) (*connect.Response[datahubv1.ListFeedLinksForExportResponse], error) {
	entries, err := h.feedLink.ListForExport(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListFeedLinksForExport failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feed links for export"))
	}

	out := make([]*datahubv1.FeedLinkExportEntry, 0, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		out = append(out, &datahubv1.FeedLinkExportEntry{Url: e.URL, Title: e.Title})
	}
	return connect.NewResponse(&datahubv1.ListFeedLinksForExportResponse{Entries: out}), nil
}

// ---------------------------------------------------------------------------
// §2.G Feed link availability
// ---------------------------------------------------------------------------

// RecordFeedLinkFailure increments the failure run and disables the link in the
// same transaction once it crosses the caller's threshold (catalog §4-4).
func (h *Handler) RecordFeedLinkFailure(ctx context.Context, req *connect.Request[datahubv1.RecordFeedLinkFailureRequest]) (*connect.Response[datahubv1.RecordFeedLinkFailureResponse], error) {
	if req.Msg.GetFeedUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	availability, disabledNow, err := h.feedLinkAvailability.RecordFailure(ctx,
		req.Msg.GetFeedUrl(), req.Msg.GetReason(), int(req.Msg.GetDisableAfterFailures()))
	if err != nil {
		h.logger.ErrorContext(ctx, "RecordFeedLinkFailure failed", "error", err, "feed_url", req.Msg.GetFeedUrl())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to record feed link failure"))
	}

	return connect.NewResponse(&datahubv1.RecordFeedLinkFailureResponse{
		Availability: feedLinkAvailabilityToProto(availability),
		DisabledNow:  disabledNow,
	}), nil
}

func (h *Handler) ResetFeedLinkFailures(ctx context.Context, req *connect.Request[datahubv1.ResetFeedLinkFailuresRequest]) (*connect.Response[datahubv1.ResetFeedLinkFailuresResponse], error) {
	if req.Msg.GetFeedUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	if err := h.feedLinkAvailability.ResetFailures(ctx, req.Msg.GetFeedUrl()); err != nil {
		h.logger.ErrorContext(ctx, "ResetFeedLinkFailures failed", "error", err, "feed_url", req.Msg.GetFeedUrl())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to reset feed link failures"))
	}
	return connect.NewResponse(&datahubv1.ResetFeedLinkFailuresResponse{}), nil
}

// ---------------------------------------------------------------------------
// §2.H Feeds
// ---------------------------------------------------------------------------

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

// maxFeedRegistrationBatch bounds one poll's worth of items. The whole batch
// is one transaction, so an unbounded request is an unbounded lock hold.
const maxFeedRegistrationBatch = 2000

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
		timestampToTimePtr(req.Msg.GetCursor()), clampFeedLimit(int(req.Msg.GetLimit())), excludes)
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

func (h *Handler) GetArticleSummaryByArticleID(ctx context.Context, req *connect.Request[datahubv1.GetArticleSummaryByArticleIDRequest]) (*connect.Response[datahubv1.GetArticleSummaryByArticleIDResponse], error) {
	if req.Msg.GetArticleId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	userID, err := optionalUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	summary, err := h.feed.GetSummaryByArticleID(ctx, req.Msg.GetArticleId(), userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetArticleSummaryByArticleID failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article summary"))
	}
	return connect.NewResponse(&datahubv1.GetArticleSummaryByArticleIDResponse{Summary: feedSummaryToProto(summary)}), nil
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

// ---------------------------------------------------------------------------
// Conversions
// ---------------------------------------------------------------------------

// feedScopeFromProto refuses the unspecified value rather than defaulting to
// ALL. The four scopes return different sets, and a caller that forgot the
// field would otherwise be handed read feeds where it asked for unread ones —
// a wrong answer that looks like a right one.
func feedScopeFromProto(s datahubv1.FeedScope) (datahub_capability_port.FeedScope, error) {
	switch s {
	case datahubv1.FeedScope_FEED_SCOPE_ALL:
		return datahub_capability_port.FeedScopeAll, nil
	case datahubv1.FeedScope_FEED_SCOPE_UNREAD:
		return datahub_capability_port.FeedScopeUnread, nil
	case datahubv1.FeedScope_FEED_SCOPE_READ:
		return datahub_capability_port.FeedScopeRead, nil
	case datahubv1.FeedScope_FEED_SCOPE_FAVORITE:
		return datahub_capability_port.FeedScopeFavorite, nil
	default:
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("scope is required"))
	}
}

func parseUUIDs(raw []string, field string) ([]uuid.UUID, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("%s contains a value that is not a uuid: %w", field, err))
		}
		out = append(out, id)
	}
	return out, nil
}

func clampFeedLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > maxFeedLimit {
		return maxFeedLimit
	}
	return limit
}

func timestampToTimePtr(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil || !ts.IsValid() {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func feedLinkAvailabilityToProto(a *domain.FeedLinkAvailability) *datahubv1.FeedLinkAvailability {
	if a == nil {
		return nil
	}
	out := &datahubv1.FeedLinkAvailability{
		FeedLinkId:          a.FeedLinkID.String(),
		IsActive:            a.IsActive,
		ConsecutiveFailures: safeconv.Int32(a.ConsecutiveFailures),
		LastFailureReason:   a.LastFailureReason,
	}
	if a.LastFailureAt != nil {
		out.LastFailureAt = timestamppb.New(*a.LastFailureAt)
	}
	return out
}

func feedSummaryToProto(s *domain.FeedSummary) *datahubv1.FeedSummary {
	if s == nil {
		return nil
	}
	return &datahubv1.FeedSummary{Summary: s.Summary}
}

func feedRowsToProto(rows []*domain.FeedRow) []*datahubv1.Feed {
	out := make([]*datahubv1.Feed, 0, len(rows))
	for _, r := range rows {
		if f := feedRowToProto(r); f != nil {
			out = append(out, f)
		}
	}
	return out
}

func feedRowToProto(r *domain.FeedRow) *datahubv1.Feed {
	if r == nil {
		return nil
	}
	// A zero pub_date stays an unset Timestamp. Many RSS items carry no
	// publication date and the driver scans the zero value; encoding it as
	// year 1 would make every such feed sort to the bottom of a client that
	// trusts the field.
	return &datahubv1.Feed{
		Id:          r.ID,
		Title:       r.Title,
		Description: r.Description,
		WebsiteUrl:  r.WebsiteURL,
		PubDate:     timeToProtoOrNil(r.PubDate),
		CreatedAt:   timeToProtoOrNil(r.CreatedAt),
		UpdatedAt:   timeToProtoOrNil(r.UpdatedAt),
		ArticleId:   r.ArticleID,
		IsRead:      r.IsRead,
		FeedLinkId:  r.FeedLinkID,
		OgImageUrl:  r.OgImageURL,
	}
}

func timeToProtoOrNil(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
