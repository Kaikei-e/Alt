package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// §2.D OG image / article_heads
// ---------------------------------------------------------------------------

// GetArticleHead returns the scraped head, leaving the field unset when the
// article has never been scraped. The caller re-scrapes on the absence, so an
// empty ArticleHead here would stop it.
func (h *Handler) GetArticleHead(ctx context.Context, req *connect.Request[datahubv1.GetArticleHeadRequest]) (*connect.Response[datahubv1.GetArticleHeadResponse], error) {
	if req.Msg.GetArticleId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	head, err := h.ogImage.GetArticleHead(ctx, req.Msg.GetArticleId())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetArticleHead failed", "error", err, "article_id", req.Msg.GetArticleId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article head"))
	}

	resp := &datahubv1.GetArticleHeadResponse{}
	if head != nil {
		resp.Head = &datahubv1.ArticleHead{
			Id:         head.ID,
			ArticleId:  head.ArticleID,
			HeadHtml:   head.HeadHTML,
			OgImageUrl: head.OgImageURL,
		}
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) BatchGetOgImageURLs(ctx context.Context, req *connect.Request[datahubv1.BatchGetOgImageURLsRequest]) (*connect.Response[datahubv1.BatchGetOgImageURLsResponse], error) {
	ids := req.Msg.GetArticleIds()
	if len(ids) == 0 {
		return connect.NewResponse(&datahubv1.BatchGetOgImageURLsResponse{OgImageUrls: map[string]string{}}), nil
	}
	if len(ids) > maxLimit {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_ids exceeds the %d id limit", maxLimit))
	}

	urls, err := h.ogImage.BatchGetOgImageURLs(ctx, ids)
	if err != nil {
		h.logger.ErrorContext(ctx, "BatchGetOgImageURLs failed", "error", err, "count", len(ids))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get og image urls"))
	}
	if urls == nil {
		urls = map[string]string{}
	}
	return connect.NewResponse(&datahubv1.BatchGetOgImageURLsResponse{OgImageUrls: urls}), nil
}

// ListFeedsMissingOgImage is gone: the batch og-image-backfill job it served
// has been replaced by resolution on demand.
//
// The procedure remains only because buf breaking uses the FILE category and
// would reject deleting it against the current main baseline. It answers
// Unimplemented rather than an empty list, because an empty list is what a
// working backfill with nothing to do looks like, and a caller that had one
// would sit there believing it was covered.
func (h *Handler) ListFeedsMissingOgImage(context.Context, *connect.Request[datahubv1.ListFeedsMissingOgImageRequest]) (*connect.Response[datahubv1.ListFeedsMissingOgImageResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented,
		errors.New("og image backfill was replaced by on-demand resolution; use GetFeedOgImageTargets and SaveFeedOgImage"))
}

func (h *Handler) ListUnwarmedOgImageURLs(ctx context.Context, req *connect.Request[datahubv1.ListUnwarmedOgImageURLsRequest]) (*connect.Response[datahubv1.ListUnwarmedOgImageURLsResponse], error) {
	urls, err := h.ogImage.ListUnwarmedOgImageURLs(ctx, clampLimit(int(req.Msg.GetLimit())))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListUnwarmedOgImageURLs failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list unwarmed og image urls"))
	}
	return connect.NewResponse(&datahubv1.ListUnwarmedOgImageURLsResponse{Urls: urls}), nil
}

func (h *Handler) PurgeExpiredArticleHeads(ctx context.Context, req *connect.Request[datahubv1.PurgeExpiredArticleHeadsRequest]) (*connect.Response[datahubv1.PurgeExpiredArticleHeadsResponse], error) {
	ttl, err := retentionFromSeconds(req.Msg.GetTtlSeconds())
	if err != nil {
		return nil, err
	}

	purged, err := h.ogImage.PurgeExpiredArticleHeads(ctx, ttl)
	if err != nil {
		h.logger.ErrorContext(ctx, "PurgeExpiredArticleHeads failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to purge article heads"))
	}
	return connect.NewResponse(&datahubv1.PurgeExpiredArticleHeadsResponse{PurgedCount: purged}), nil
}

// GetFeedOgImageTargets answers whether an origin request is warranted for each
// feed a reader has brought into view.
//
// The limit is the same maxLimit the other batch reads use. It is not a
// pagination knob: it bounds how many origins one viewport change can cause the
// caller to contact.
func (h *Handler) GetFeedOgImageTargets(ctx context.Context, req *connect.Request[datahubv1.GetFeedOgImageTargetsRequest]) (*connect.Response[datahubv1.GetFeedOgImageTargetsResponse], error) {
	ids := req.Msg.GetFeedIds()
	if len(ids) == 0 {
		return connect.NewResponse(&datahubv1.GetFeedOgImageTargetsResponse{}), nil
	}
	if len(ids) > maxLimit {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_ids exceeds the %d id limit", maxLimit))
	}

	targets, err := h.ogImage.GetFeedOgImageTargets(ctx, ids)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetFeedOgImageTargets failed", "error", err, "count", len(ids))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get feed og image targets"))
	}

	out := make([]*datahubv1.FeedOgImageTarget, 0, len(targets))
	for _, t := range targets {
		out = append(out, &datahubv1.FeedOgImageTarget{
			FeedId:     t.FeedID,
			PageUrl:    t.PageURL,
			OgImageUrl: t.OgImageURL,
			Suppressed: t.Suppressed,
			// attempts and retry_after_seconds are what let the caller escalate
			// and what let it answer a reader's client with a number rather than
			// "never". Both are held only here, so dropping either from this
			// mapping is silent: the caller keeps working and simply restarts
			// the ladder — or gives the card up — on every ask.
			Attempts:          safeconv.Int32(t.Attempts),
			RetryAfterSeconds: t.RetryAfterSeconds,
		})
	}
	return connect.NewResponse(&datahubv1.GetFeedOgImageTargetsResponse{Targets: out}), nil
}

// SaveFeedOgImage records one resolution outcome.
//
// retry_after_seconds of zero is stored as "not within this retention window"
// rather than as "retry now". That is the distinction the whole design rests
// on: a robots.txt disallow answered with an immediate retry would put the
// request back on the origin at the next scroll.
func (h *Handler) SaveFeedOgImage(ctx context.Context, req *connect.Request[datahubv1.SaveFeedOgImageRequest]) (*connect.Response[datahubv1.SaveFeedOgImageResponse], error) {
	feedID := req.Msg.GetFeedId()
	if feedID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_id is required"))
	}

	imageURL := req.Msg.GetOgImageUrl()
	refusal := domain.OgImageRefusal(req.Msg.GetReason())
	if imageURL == "" && refusal == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("a resolution must carry either og_image_url or reason"))
	}

	retryAfter := time.Duration(req.Msg.GetRetryAfterSeconds()) * time.Second
	if err := h.ogImage.SaveFeedOgImage(ctx, feedID, imageURL, refusal, retryAfter); err != nil {
		h.logger.ErrorContext(ctx, "SaveFeedOgImage failed", "error", err, "feed_id", feedID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save feed og image"))
	}
	return connect.NewResponse(&datahubv1.SaveFeedOgImageResponse{}), nil
}

func (h *Handler) PurgeExpiredFeedOgImages(ctx context.Context, req *connect.Request[datahubv1.PurgeExpiredFeedOgImagesRequest]) (*connect.Response[datahubv1.PurgeExpiredFeedOgImagesResponse], error) {
	ttl, err := retentionFromSeconds(req.Msg.GetTtlSeconds())
	if err != nil {
		return nil, err
	}

	purged, err := h.ogImage.PurgeExpiredFeedOgImages(ctx, ttl)
	if err != nil {
		h.logger.ErrorContext(ctx, "PurgeExpiredFeedOgImages failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to purge feed og images"))
	}
	return connect.NewResponse(&datahubv1.PurgeExpiredFeedOgImagesResponse{PurgedCount: purged}), nil
}

// ---------------------------------------------------------------------------
// §2.E Image proxy cache
// ---------------------------------------------------------------------------

func (h *Handler) GetImageProxyCache(ctx context.Context, req *connect.Request[datahubv1.GetImageProxyCacheRequest]) (*connect.Response[datahubv1.GetImageProxyCacheResponse], error) {
	if req.Msg.GetUrlHash() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url_hash is required"))
	}

	entry, err := h.imageProxyCache.Get(ctx, req.Msg.GetUrlHash())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetImageProxyCache failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get image proxy cache"))
	}

	resp := &datahubv1.GetImageProxyCacheResponse{}
	if entry != nil {
		resp.Entry = imageProxyCacheEntryToProto(entry)
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) PutImageProxyCache(ctx context.Context, req *connect.Request[datahubv1.PutImageProxyCacheRequest]) (*connect.Response[datahubv1.PutImageProxyCacheResponse], error) {
	msg := req.Msg.GetEntry()
	if msg == nil || msg.GetUrlHash() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("entry.url_hash is required"))
	}

	entry := &domain.ImageProxyCacheEntry{
		URLHash:     msg.GetUrlHash(),
		OriginalURL: msg.GetOriginalUrl(),
		Data:        msg.GetData(),
		ContentType: msg.GetContentType(),
		Width:       int(msg.GetWidth()),
		Height:      int(msg.GetHeight()),
		SizeBytes:   int(msg.GetSizeBytes()),
		ETag:        msg.GetEtag(),
		CreatedAt:   timeOrZero(msg.GetCreatedAt()),
		ExpiresAt:   timeOrZero(msg.GetExpiresAt()),
	}
	if err := h.imageProxyCache.Put(ctx, entry); err != nil {
		h.logger.ErrorContext(ctx, "PutImageProxyCache failed", "error", err, "url_hash", entry.URLHash)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save image proxy cache"))
	}
	return connect.NewResponse(&datahubv1.PutImageProxyCacheResponse{}), nil
}

func (h *Handler) EvictExpiredImageProxyCache(ctx context.Context, _ *connect.Request[datahubv1.EvictExpiredImageProxyCacheRequest]) (*connect.Response[datahubv1.EvictExpiredImageProxyCacheResponse], error) {
	evicted, err := h.imageProxyCache.EvictExpired(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "EvictExpiredImageProxyCache failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to evict expired image cache"))
	}
	return connect.NewResponse(&datahubv1.EvictExpiredImageProxyCacheResponse{EvictedCount: evicted}), nil
}

func (h *Handler) PurgeImageProxyCacheOlderThan(ctx context.Context, req *connect.Request[datahubv1.PurgeImageProxyCacheOlderThanRequest]) (*connect.Response[datahubv1.PurgeImageProxyCacheOlderThanResponse], error) {
	ttl, err := retentionFromSeconds(req.Msg.GetTtlSeconds())
	if err != nil {
		return nil, err
	}

	purged, err := h.imageProxyCache.PurgeOlderThan(ctx, ttl)
	if err != nil {
		h.logger.ErrorContext(ctx, "PurgeImageProxyCacheOlderThan failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to purge image cache"))
	}
	return connect.NewResponse(&datahubv1.PurgeImageProxyCacheOlderThanResponse{PurgedCount: purged}), nil
}

func imageProxyCacheEntryToProto(e *domain.ImageProxyCacheEntry) *datahubv1.ImageProxyCacheEntry {
	return &datahubv1.ImageProxyCacheEntry{
		UrlHash:     e.URLHash,
		OriginalUrl: e.OriginalURL,
		Data:        e.Data,
		ContentType: e.ContentType,
		Width:       safeconv.Int32(e.Width),
		Height:      safeconv.Int32(e.Height),
		SizeBytes:   int64(e.SizeBytes),
		Etag:        e.ETag,
		CreatedAt:   timestampOrNil(e.CreatedAt),
		ExpiresAt:   timestampOrNil(e.ExpiresAt),
	}
}

// SaveArticleHead upserts the scraped head. Served by the OG image port —
// see WithArticleCapabilities.
func (h *Handler) SaveArticleHead(ctx context.Context, req *connect.Request[datahubv1.SaveArticleHeadRequest]) (*connect.Response[datahubv1.SaveArticleHeadResponse], error) {
	if req.Msg.GetArticleId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	if err := h.ogImage.SaveArticleHead(ctx, req.Msg.GetArticleId(), req.Msg.GetHeadHtml(), req.Msg.GetOgImageUrl()); err != nil {
		h.logger.ErrorContext(ctx, "SaveArticleHead failed", "error", err, "article_id", req.Msg.GetArticleId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save article head"))
	}
	return connect.NewResponse(&datahubv1.SaveArticleHeadResponse{}), nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// retentionFromSeconds converts a retention window and rejects a non-positive
// one at the delivery layer.
//
// An omitted protobuf field and an explicit zero are the same bytes, so a
// provider that accepted zero would let a caller that forgot the field delete
// every row the retention query matches — for PurgeExpiredArticleHeads, the
// entire table.
func retentionFromSeconds(seconds int64) (time.Duration, error) {
	if seconds <= 0 {
		return 0, connect.NewError(connect.CodeInvalidArgument,
			errors.New("ttl_seconds must be positive: an omitted field and an explicit zero are indistinguishable on the wire, "+
				"and zero would purge every row"))
	}
	return time.Duration(seconds) * time.Second, nil
}

func timeOrZero(ts *timestamppb.Timestamp) time.Time {
	if ts == nil || !ts.IsValid() {
		return time.Time{}
	}
	return ts.AsTime()
}
