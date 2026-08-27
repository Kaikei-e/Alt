package datahub_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
)

// OgImageGateway covers article_heads: the scraped <head> of an article page
// and the og:image URL extracted from it (catalog §2.D).
//
// The scraping itself stays in alt-harvester — it is an external HTTP fetch,
// which ADR-000954 D4 keeps on the calling side. Only the reads and the
// retention delete cross this boundary.
type OgImageGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewOgImageGateway(client datahubv1connect.DataHubServiceClient) *OgImageGateway {
	if client == nil {
		panic("datahub_gateway: OgImageGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &OgImageGateway{client: client}
}

// SaveArticleHead stores the scraped head and the og:image URL extracted from
// it (catalog §2.B W3-B2).
//
// head_html is NOT NULL in the table, so a caller with nothing to store sends
// a `<head></head>` placeholder — the og-image backfill job does exactly that
// when it has an image but no markup worth keeping.
func (g *OgImageGateway) SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error {
	_, err := g.client.SaveArticleHead(ctx, connect.NewRequest(&datahubv1.SaveArticleHeadRequest{
		ArticleId:  articleID,
		HeadHtml:   headHTML,
		OgImageUrl: ogImageURL,
	}))
	if err != nil {
		return fmt.Errorf("save article head %s: %w", articleID, err)
	}
	return nil
}

// FetchArticleHeadByArticleID returns the stored head record, or (nil, nil)
// when the article has never been scraped.
//
// The nil-without-error shape is inherited from the driver it replaces and is
// load-bearing: fetch_article_usecase distinguishes "no row, go scrape" from
// "scraped, found nothing", and an error would collapse both into a failure.
func (g *OgImageGateway) FetchArticleHeadByArticleID(ctx context.Context, articleID string) (*domain.ArticleHead, error) {
	resp, err := g.client.GetArticleHead(ctx, connect.NewRequest(&datahubv1.GetArticleHeadRequest{
		ArticleId: articleID,
	}))
	if err != nil {
		return nil, fmt.Errorf("get article head %s: %w", articleID, err)
	}
	return articleHeadFromProto(resp.Msg.GetHead()), nil
}

// FetchOgImageURLsByArticleIDs resolves og:image URLs for many articles.
// Article ids with no row, or a row with no image, are absent from the map.
func (g *OgImageGateway) FetchOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error) {
	if len(articleIDs) == 0 {
		// Saves a round trip on a call the trail thumbnail path makes for
		// every rendered page, including empty ones.
		return map[string]string{}, nil
	}

	resp, err := g.client.BatchGetOgImageURLs(ctx, connect.NewRequest(&datahubv1.BatchGetOgImageURLsRequest{
		ArticleIds: articleIDs,
	}))
	if err != nil {
		return nil, fmt.Errorf("batch get og image urls (%d ids): %w", len(articleIDs), err)
	}

	urls := resp.Msg.GetOgImageUrls()
	if urls == nil {
		return map[string]string{}, nil
	}
	return urls, nil
}

// FetchOgImageURLByArticleID is the single-article read, served by the batch
// capability rather than one of its own.
//
// Catalog §2.D lists the single and batch driver methods separately, but the
// difference between them is a loop, not a transaction or an invariant — the
// thing a capability is supposed to draw a line around (ADR-000954 D3). Two
// procedures returning the same column would be two things to keep in step for
// no gain.
func (g *OgImageGateway) FetchOgImageURLByArticleID(ctx context.Context, articleID string) (string, error) {
	urls, err := g.FetchOgImageURLsByArticleIDs(ctx, []string{articleID})
	if err != nil {
		return "", err
	}
	return urls[articleID], nil
}

// FetchFeedOgImageTargets returns, for feeds a reader has brought into view,
// the page that would be fetched and whether an earlier attempt already
// settled the question.
func (g *OgImageGateway) FetchFeedOgImageTargets(ctx context.Context, feedIDs []string) ([]domain.FeedOgImageTarget, error) {
	if len(feedIDs) == 0 {
		return nil, nil
	}

	resp, err := g.client.GetFeedOgImageTargets(ctx, connect.NewRequest(&datahubv1.GetFeedOgImageTargetsRequest{
		FeedIds: feedIDs,
	}))
	if err != nil {
		return nil, fmt.Errorf("get feed og image targets (%d ids): %w", len(feedIDs), err)
	}

	protoTargets := resp.Msg.GetTargets()
	targets := make([]domain.FeedOgImageTarget, 0, len(protoTargets))
	for _, t := range protoTargets {
		targets = append(targets, domain.FeedOgImageTarget{
			FeedID:     t.GetFeedId(),
			PageURL:    t.GetPageUrl(),
			OgImageURL: t.GetOgImageUrl(),
			Suppressed: t.GetSuppressed(),
			// Both counters cross unchanged. Attempts is the raw stored count,
			// zero for a feed with no row, and RetryAfter normalises that at the
			// point of use rather than here — see domain.FeedOgImageTarget.
			Attempts:          int(t.GetAttempts()),
			RetryAfterSeconds: t.GetRetryAfterSeconds(),
		})
	}
	return targets, nil
}

// SaveFeedOgImage records one resolution outcome. An empty ogImageURL means the
// origin refused, and `refusal` says why; that record is what stops the next
// reader scrolling past the same card from causing the same request.
//
// retryAfter is passed in rather than derived here, which reverses the earlier
// arrangement. The window is still a property of *why* the origin said no —
// nobody picks a number freely, they call
// domain.OgImageRefusal.RetryAfter(attempts) — but it now also depends on how
// many attempts this feed has already cost, and the caller is the only place
// that holds both halves. It also has to report the same number to the reader's
// client in the same breath, so deriving it twice from two different attempt
// counts would let the bar the client waits out and the bar the row records
// drift apart.
//
// A successful resolution passes zero, which is stored as "no bar". Asking an
// empty refusal for its window would return the catch-all instead — a bar
// attached to a row that resolved fine.
func (g *OgImageGateway) SaveFeedOgImage(
	ctx context.Context,
	feedID, ogImageURL string,
	refusal domain.OgImageRefusal,
	retryAfter time.Duration,
) error {
	var retryAfterSeconds int64
	if refusal != "" && retryAfter > 0 {
		retryAfterSeconds = int64(retryAfter.Seconds())
	}

	_, err := g.client.SaveFeedOgImage(ctx, connect.NewRequest(&datahubv1.SaveFeedOgImageRequest{
		FeedId:            feedID,
		OgImageUrl:        ogImageURL,
		Reason:            string(refusal),
		RetryAfterSeconds: retryAfterSeconds,
	}))
	if err != nil {
		return fmt.Errorf("save feed og image %s: %w", feedID, err)
	}
	return nil
}

// CleanupExpiredFeedOgImages enforces the feed_og_images copyright retention
// window and returns how many rows went.
func (g *OgImageGateway) CleanupExpiredFeedOgImages(ctx context.Context, ttl time.Duration) (int64, error) {
	resp, err := g.client.PurgeExpiredFeedOgImages(ctx, connect.NewRequest(&datahubv1.PurgeExpiredFeedOgImagesRequest{
		TtlSeconds: int64(ttl.Seconds()),
	}))
	if err != nil {
		return 0, fmt.Errorf("purge feed og images older than %s: %w", ttl, err)
	}
	return resp.Msg.GetPurgedCount(), nil
}

// FetchUnwarmedOgImageURLs returns feed og:image URLs with no live cache entry.
func (g *OgImageGateway) FetchUnwarmedOgImageURLs(ctx context.Context, limit int) ([]string, error) {
	resp, err := g.client.ListUnwarmedOgImageURLs(ctx, connect.NewRequest(&datahubv1.ListUnwarmedOgImageURLsRequest{
		Limit: safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("list unwarmed og image urls: %w", err)
	}
	return resp.Msg.GetUrls(), nil
}

// CleanupExpiredArticleHeads enforces the article_heads copyright retention
// window and returns how many rows went.
func (g *OgImageGateway) CleanupExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error) {
	resp, err := g.client.PurgeExpiredArticleHeads(ctx, connect.NewRequest(&datahubv1.PurgeExpiredArticleHeadsRequest{
		TtlSeconds: int64(ttl.Seconds()),
	}))
	if err != nil {
		return 0, fmt.Errorf("purge article heads older than %s: %w", ttl, err)
	}
	return resp.Msg.GetPurgedCount(), nil
}
