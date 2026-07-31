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
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// maxTagLimit and defaultTagLimit bound the tag walks. The ceiling is the
// provider's rather than each caller's, because it is a statement about what
// this database will plan for; the default is what the in-process callers
// passed when they omitted one.
const (
	maxTagLimit     = 500
	defaultTagLimit = 100
)

// maxReadFeedIDsPerRequest bounds the batch GetReadFeedIDs accepts.
//
// The read is an overlay on one page of feeds, so the realistic input is a
// page's worth of ids. The limit exists because the ids become a Postgres
// array literal: an unbounded list is an unbounded query string, and the
// caller that sends one is buggy rather than ambitious.
const maxReadFeedIDsPerRequest = 1000

// WithWave3Batch4Capabilities wires the per-user feed state and the tag reads
// ADR-000954 Wave 3 batch 4 moved out of alt-backend (capability catalog §2.I
// / §2.J).
//
// Nil panics, as in every batch before it. After this batch these procedures
// are the only route to read_status, user_feed_subscriptions, favorite_feeds
// and the tag tables. A data hub started with a nil read-state port would
// answer MarkFeedRead with Unimplemented — indistinguishable from a retired
// procedure — and every reader would find their read marks silently
// discarded while health checks stayed green (CLAUDE.md rule 8, ADR-000928).
func WithWave3Batch4Capabilities(
	readState datahub_capability_port.ReadStatePort,
	tagRead datahub_capability_port.TagReadPort,
) HandlerOption {
	switch {
	case readState == nil:
		panic("datahubapi: ReadStatePort is required — read marks, subscriptions and favourites have no other route to their tables")
	case tagRead == nil:
		panic("datahubapi: TagReadPort is required — every tag surface, including the on-the-fly generation path, reads through it")
	}

	return func(h *Handler) {
		h.readState = readState
		h.tagRead = tagRead
	}
}

// ---------------------------------------------------------------------------
// §2.I Read state / subscriptions / favorites
// ---------------------------------------------------------------------------

// MarkFeedRead and MarkArticleRead answer NotFound for a URL with no feeds
// row, and that is one decision made in one place (capability catalog §4-5).
//
// The two procedures reach the same table by two different keys, so they stay
// separate; what they no longer differ in is how "there is no such feed" is
// detected and reported. domain.ErrFeedNotFound arrives from a single upsert's
// RowsAffected() == 0 in both, and both map it here. A consumer sees only the
// Connect code, so had the two kept their old detections — one pgx.ErrNoRows
// from a preceding SELECT, one RowsAffected() — the same code would have meant
// two different things depending on which procedure produced it.
func (h *Handler) MarkFeedRead(ctx context.Context, req *connect.Request[datahubv1.MarkFeedReadRequest]) (*connect.Response[datahubv1.MarkFeedReadResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetFeedUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	if err := h.readState.MarkFeedRead(ctx, req.Msg.GetFeedUrl(), userID); err != nil {
		return nil, h.readStateWriteError(ctx, "MarkFeedRead", err, "feed_url", req.Msg.GetFeedUrl())
	}
	return connect.NewResponse(&datahubv1.MarkFeedReadResponse{}), nil
}

func (h *Handler) MarkArticleRead(ctx context.Context, req *connect.Request[datahubv1.MarkArticleReadRequest]) (*connect.Response[datahubv1.MarkArticleReadResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetArticleUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_url is required"))
	}

	if err := h.readState.MarkArticleRead(ctx, req.Msg.GetArticleUrl(), userID); err != nil {
		return nil, h.readStateWriteError(ctx, "MarkArticleRead", err, "article_url", req.Msg.GetArticleUrl())
	}
	return connect.NewResponse(&datahubv1.MarkArticleReadResponse{}), nil
}

// readStateWriteError is the single mapping from the two absence sentinels the
// §2.I writes raise to Connect codes.
//
// domain.ErrFeedNotFound comes from the read-status writes, pgx.ErrNoRows from
// the favourite writes, and both mean the same thing to a consumer: the URL
// names nothing. Every other error is Internal with the detail kept in the log
// rather than in the response, because the caller is another service and the
// text would only travel to a place that cannot act on it.
func (h *Handler) readStateWriteError(ctx context.Context, procedure string, err error, urlKey, urlValue string) error {
	if errors.Is(err, domain.ErrFeedNotFound) || errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("feed not found"))
	}
	h.logger.ErrorContext(ctx, procedure+" failed", "error", err, urlKey, urlValue)
	return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to %s", procedure))
}

func (h *Handler) GetReadFeedIDs(ctx context.Context, req *connect.Request[datahubv1.GetReadFeedIDsRequest]) (*connect.Response[datahubv1.GetReadFeedIDsResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	if len(req.Msg.GetFeedIds()) > maxReadFeedIDsPerRequest {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_ids exceeds the %d entry limit", maxReadFeedIDsPerRequest))
	}

	feedIDs, err := parseUUIDs(req.Msg.GetFeedIds(), "feed_ids")
	if err != nil {
		return nil, err
	}
	// An empty request is answered without a query. The caller sends it when a
	// feed page came back empty, and the answer cannot be anything but empty.
	if len(feedIDs) == 0 {
		return connect.NewResponse(&datahubv1.GetReadFeedIDsResponse{}), nil
	}

	read, err := h.readState.ReadFeedIDs(ctx, userID, feedIDs)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetReadFeedIDs failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get read feed ids"))
	}
	return connect.NewResponse(&datahubv1.GetReadFeedIDsResponse{ReadFeedIds: uuidStrings(read)}), nil
}

func (h *Handler) GetAllReadFeedIDs(ctx context.Context, req *connect.Request[datahubv1.GetAllReadFeedIDsRequest]) (*connect.Response[datahubv1.GetAllReadFeedIDsResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	read, err := h.readState.AllReadFeedIDs(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetAllReadFeedIDs failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get all read feed ids"))
	}
	return connect.NewResponse(&datahubv1.GetAllReadFeedIDsResponse{ReadFeedIds: uuidStrings(read)}), nil
}

func (h *Handler) GetUserSubscribedFeedLinkIDs(ctx context.Context, req *connect.Request[datahubv1.GetUserSubscribedFeedLinkIDsRequest]) (*connect.Response[datahubv1.GetUserSubscribedFeedLinkIDsResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	ids, err := h.readState.SubscribedFeedLinkIDs(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetUserSubscribedFeedLinkIDs failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get subscribed feed link ids"))
	}
	return connect.NewResponse(&datahubv1.GetUserSubscribedFeedLinkIDsResponse{FeedLinkIds: uuidStrings(ids)}), nil
}

func (h *Handler) ListSubscriptions(ctx context.Context, req *connect.Request[datahubv1.ListSubscriptionsRequest]) (*connect.Response[datahubv1.ListSubscriptionsResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	sources, err := h.readState.ListSubscriptions(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListSubscriptions failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list subscriptions"))
	}

	out := make([]*datahubv1.FeedSubscription, 0, len(sources))
	for _, s := range sources {
		if s == nil {
			continue
		}
		sub := &datahubv1.FeedSubscription{
			FeedLinkId:   s.ID,
			Url:          s.URL,
			IsSubscribed: s.IsSubscribed,
		}
		// subscribedAt is sent only for a followed link. The query coalesces
		// the missing join to now(), so an unfollowed row carries a timestamp
		// that means nothing; forwarding it would have the screen show a
		// follow date for something nobody follows.
		if s.IsSubscribed && !s.CreatedAt.IsZero() {
			sub.SubscribedAt = timestamppb.New(s.CreatedAt)
		}
		out = append(out, sub)
	}
	return connect.NewResponse(&datahubv1.ListSubscriptionsResponse{Subscriptions: out}), nil
}

func (h *Handler) Subscribe(ctx context.Context, req *connect.Request[datahubv1.SubscribeRequest]) (*connect.Response[datahubv1.SubscribeResponse], error) {
	userID, feedLinkID, err := userAndFeedLinkID(req.Msg.GetUserId(), req.Msg.GetFeedLinkId())
	if err != nil {
		return nil, err
	}

	if err := h.readState.Subscribe(ctx, userID, feedLinkID); err != nil {
		h.logger.ErrorContext(ctx, "Subscribe failed", "error", err, "user_id", userID, "feed_link_id", feedLinkID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to subscribe"))
	}
	return connect.NewResponse(&datahubv1.SubscribeResponse{}), nil
}

func (h *Handler) Unsubscribe(ctx context.Context, req *connect.Request[datahubv1.UnsubscribeRequest]) (*connect.Response[datahubv1.UnsubscribeResponse], error) {
	userID, feedLinkID, err := userAndFeedLinkID(req.Msg.GetUserId(), req.Msg.GetFeedLinkId())
	if err != nil {
		return nil, err
	}

	if err := h.readState.Unsubscribe(ctx, userID, feedLinkID); err != nil {
		h.logger.ErrorContext(ctx, "Unsubscribe failed", "error", err, "user_id", userID, "feed_link_id", feedLinkID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to unsubscribe"))
	}
	return connect.NewResponse(&datahubv1.UnsubscribeResponse{}), nil
}

func (h *Handler) AddFavoriteFeed(ctx context.Context, req *connect.Request[datahubv1.AddFavoriteFeedRequest]) (*connect.Response[datahubv1.AddFavoriteFeedResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetFeedUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	if err := h.readState.AddFavorite(ctx, req.Msg.GetFeedUrl(), userID); err != nil {
		return nil, h.readStateWriteError(ctx, "AddFavoriteFeed", err, "feed_url", req.Msg.GetFeedUrl())
	}
	return connect.NewResponse(&datahubv1.AddFavoriteFeedResponse{}), nil
}

func (h *Handler) RemoveFavoriteFeed(ctx context.Context, req *connect.Request[datahubv1.RemoveFavoriteFeedRequest]) (*connect.Response[datahubv1.RemoveFavoriteFeedResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetFeedUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	if err := h.readState.RemoveFavorite(ctx, req.Msg.GetFeedUrl(), userID); err != nil {
		return nil, h.readStateWriteError(ctx, "RemoveFavoriteFeed", err, "feed_url", req.Msg.GetFeedUrl())
	}
	return connect.NewResponse(&datahubv1.RemoveFavoriteFeedResponse{}), nil
}

// ---------------------------------------------------------------------------
// §2.J Tag reads
// ---------------------------------------------------------------------------

func (h *Handler) GetArticleTags(ctx context.Context, req *connect.Request[datahubv1.GetArticleTagsRequest]) (*connect.Response[datahubv1.GetArticleTagsResponse], error) {
	if req.Msg.GetArticleId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	tags, err := h.tagRead.ArticleTags(ctx, req.Msg.GetArticleId())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetArticleTags failed", "error", err, "article_id", req.Msg.GetArticleId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article tags"))
	}
	// An untagged article answers an empty list, not NotFound. The caller
	// treats emptiness as "ask mq-hub to generate some"; an error would make an
	// untagged article look like a database fault and suppress the generation.
	return connect.NewResponse(&datahubv1.GetArticleTagsResponse{Tags: feedTagsToProto(tags)}), nil
}

func (h *Handler) GetFeedTags(ctx context.Context, req *connect.Request[datahubv1.GetFeedTagsRequest]) (*connect.Response[datahubv1.GetFeedTagsResponse], error) {
	if req.Msg.GetFeedId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_id is required"))
	}
	limit := clampTagLimit(req.Msg.GetLimit())

	var cursor *time.Time
	if c := req.Msg.GetCursor(); c != nil {
		t := c.AsTime()
		cursor = &t
	}

	tags, err := h.tagRead.FeedTags(ctx, req.Msg.GetFeedId(), cursor, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetFeedTags failed", "error", err, "feed_id", req.Msg.GetFeedId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get feed tags"))
	}
	return connect.NewResponse(&datahubv1.GetFeedTagsResponse{Tags: feedTagsToProto(tags)}), nil
}

func (h *Handler) GetTagCooccurrences(ctx context.Context, req *connect.Request[datahubv1.GetTagCooccurrencesRequest]) (*connect.Response[datahubv1.GetTagCooccurrencesResponse], error) {
	names := req.Msg.GetTagNames()
	if len(names) == 0 {
		return connect.NewResponse(&datahubv1.GetTagCooccurrencesResponse{}), nil
	}
	if len(names) > maxTagLimit {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("tag_names exceeds the %d entry limit", maxTagLimit))
	}

	items, err := h.tagRead.Cooccurrences(ctx, names)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetTagCooccurrences failed", "error", err, "tag_count", len(names))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get tag cooccurrences"))
	}

	out := make([]*datahubv1.TagCooccurrence, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, &datahubv1.TagCooccurrence{
			TagNameA:    it.TagNameA,
			TagNameB:    it.TagNameB,
			SharedCount: safeconv.Int32(it.SharedCount),
		})
	}
	return connect.NewResponse(&datahubv1.GetTagCooccurrencesResponse{Cooccurrences: out}), nil
}

func (h *Handler) SearchTagsByPrefix(ctx context.Context, req *connect.Request[datahubv1.SearchTagsByPrefixRequest]) (*connect.Response[datahubv1.SearchTagsByPrefixResponse], error) {
	if req.Msg.GetPrefix() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("prefix is required"))
	}
	limit := clampTagLimit(req.Msg.GetLimit())

	hits, err := h.tagRead.SearchByPrefix(ctx, req.Msg.GetPrefix(), limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "SearchTagsByPrefix failed", "error", err, "prefix", req.Msg.GetPrefix())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to search tags by prefix"))
	}

	out := make([]*datahubv1.TagPrefixHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, &datahubv1.TagPrefixHit{
			TagName:      hit.TagName,
			ArticleCount: safeconv.Int32(hit.ArticleCount),
		})
	}
	return connect.NewResponse(&datahubv1.SearchTagsByPrefixResponse{Hits: out}), nil
}

func (h *Handler) GetTagArticleCounts(ctx context.Context, req *connect.Request[datahubv1.GetTagArticleCountsRequest]) (*connect.Response[datahubv1.GetTagArticleCountsResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	// `since` is required rather than defaulted. Without it the query counts
	// the user's entire history, which is a different and far more expensive
	// question than the one every caller asks; defaulting would answer it
	// silently.
	if req.Msg.GetSince() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("since is required"))
	}

	counts, err := h.tagRead.ArticleCounts(ctx, userID, req.Msg.GetSince().AsTime())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetTagArticleCounts failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get tag article counts"))
	}

	out := make([]*datahubv1.TagArticleCount, 0, len(counts))
	for _, c := range counts {
		out = append(out, &datahubv1.TagArticleCount{
			TagName:      c.TagName,
			ArticleCount: safeconv.Int32(c.ArticleCount),
		})
	}
	return connect.NewResponse(&datahubv1.GetTagArticleCountsResponse{Counts: out}), nil
}

// ---------------------------------------------------------------------------
// Shared argument handling
// ---------------------------------------------------------------------------

// requiredUserID rejects a missing or malformed tenant rather than falling
// back to a zero UUID.
//
// Every §2.I procedure is tenant-scoped and none of them has a meaningful
// unscoped form: a zero user id would silently read or write the state of a
// user that does not exist, which is a successful-looking no-op rather than an
// error anyone would notice.
func requiredUserID(raw string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id is required"))
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id must be a uuid: %w", err))
	}
	return id, nil
}

func userAndFeedLinkID(rawUser, rawFeedLink string) (uuid.UUID, uuid.UUID, error) {
	userID, err := requiredUserID(rawUser)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if rawFeedLink == "" {
		return uuid.Nil, uuid.Nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_link_id is required"))
	}
	feedLinkID, err := uuid.Parse(rawFeedLink)
	if err != nil {
		return uuid.Nil, uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("feed_link_id must be a uuid: %w", err))
	}
	return userID, feedLinkID, nil
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

// clampTagLimit applies the provider's page ceiling, matching clampFeedLimit
// in batch 3 rather than introducing a second convention for the same
// question. An unset or non-positive limit gets the default the in-process
// callers passed.
func clampTagLimit(limit int32) int {
	if limit <= 0 {
		return defaultTagLimit
	}
	if int(limit) > maxTagLimit {
		return maxTagLimit
	}
	return int(limit)
}

func feedTagsToProto(tags []*domain.FeedTag) []*datahubv1.FeedTag {
	out := make([]*datahubv1.FeedTag, 0, len(tags))
	for _, t := range tags {
		if t == nil {
			continue
		}
		pb := &datahubv1.FeedTag{
			Id:         t.ID,
			FeedId:     t.FeedID,
			TagName:    t.TagName,
			Confidence: t.Confidence,
			TagType:    t.TagType,
		}
		// Zero times stay absent rather than becoming 1970. The article-tag
		// read does not select feed_id or updated_at, and a consumer that saw
		// an epoch timestamp would sort on it.
		if !t.CreatedAt.IsZero() {
			pb.CreatedAt = timestamppb.New(t.CreatedAt)
		}
		if !t.UpdatedAt.IsZero() {
			pb.UpdatedAt = timestamppb.New(t.UpdatedAt)
		}
		out = append(out, pb)
	}
	return out
}
