package datahub_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
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

// FetchFeedsMissingOgImage returns the og-image-backfill work list.
func (g *OgImageGateway) FetchFeedsMissingOgImage(ctx context.Context, limit int) ([]domain.OgImageBackfillCandidate, error) {
	resp, err := g.client.ListFeedsMissingOgImage(ctx, connect.NewRequest(&datahubv1.ListFeedsMissingOgImageRequest{
		Limit: safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("list feeds missing og image: %w", err)
	}

	candidates := make([]domain.OgImageBackfillCandidate, 0, len(resp.Msg.GetCandidates()))
	for _, c := range resp.Msg.GetCandidates() {
		candidates = append(candidates, domain.OgImageBackfillCandidate{
			ArticleID: c.GetArticleId(),
			URL:       c.GetUrl(),
		})
	}
	return candidates, nil
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
