package job

import (
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/shared/driver/alt_db"
	"alt/utils/html_parser"
	"context"
	"fmt"
	"log/slog"
)

// ogBackfillBatchLimit bounds how many article pages are scraped per run, to
// cap external HTTP load. Per-host rate limiting is enforced by the fetcher.
const ogBackfillBatchLimit = 50

// ogBackfillCandidateLister abstracts the work-list query (for testability).
type ogBackfillCandidateLister interface {
	FetchFeedsMissingOgImage(ctx context.Context, limit int) ([]alt_db.OgBackfillCandidate, error)
}

// articleContentFetcher abstracts the SSRF-protected, rate-limited article page
// fetch (for testability).
type articleContentFetcher interface {
	FetchArticleContents(ctx context.Context, articleURL string) (*string, error)
}

// articleHeadSaver abstracts persisting the scraped head + og:image.
type articleHeadSaver interface {
	SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error
}

// OgImageBackfillJob returns a JobScheduler function that scrapes og:image for
// recent feeds that have no image yet, persists it to article_heads, and warms
// the image proxy cache. Coverage complement to the OGP image warmer, which
// only warms feeds that already have an og_image_url.
func OgImageBackfillJob(
	r *alt_db.AltDBRepository,
	fetcher articleContentFetcher,
	imageProxy *image_proxy_usecase.ImageProxyUsecase,
) func(ctx context.Context) error {
	if r == nil || fetcher == nil || imageProxy == nil {
		return func(ctx context.Context) error {
			slog.InfoContext(ctx, "og-image-backfill disabled: dependencies not wired")
			return nil
		}
	}
	return ogImageBackfillJobFn(r, fetcher, r, imageProxy)
}

func ogImageBackfillJobFn(
	lister ogBackfillCandidateLister,
	fetcher articleContentFetcher,
	saver articleHeadSaver,
	warmer imageWarmer,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		candidates, err := lister.FetchFeedsMissingOgImage(ctx, ogBackfillBatchLimit)
		if err != nil {
			return fmt.Errorf("fetch og backfill candidates: %w", err)
		}
		if len(candidates) == 0 {
			slog.InfoContext(ctx, "og-image-backfill: no candidates")
			return nil
		}

		slog.InfoContext(ctx, "og-image-backfill: starting", "candidates", len(candidates))

		backfilled := 0
		for _, c := range candidates {
			if ctx.Err() != nil {
				slog.InfoContext(ctx, "og-image-backfill: context cancelled, stopping early", "backfilled", backfilled)
				return nil
			}

			htmlPtr, err := fetcher.FetchArticleContents(ctx, c.URL)
			if err != nil {
				slog.WarnContext(ctx, "og-image-backfill: fetch failed", "url", c.URL, "error", err)
				continue
			}
			if htmlPtr == nil || *htmlPtr == "" {
				continue
			}

			ogImage := html_parser.ExtractOgImageURL(*htmlPtr, c.URL)
			if ogImage == "" {
				continue
			}

			headHTML := html_parser.ExtractHead(*htmlPtr)
			if headHTML == "" {
				// article_heads.head_html is NOT NULL; keep a minimal placeholder
				// so the og:image row can still be stored.
				headHTML = "<head></head>"
			}

			if err := saver.SaveArticleHead(ctx, c.ArticleID, headHTML, ogImage); err != nil {
				slog.WarnContext(ctx, "og-image-backfill: save failed", "article_id", c.ArticleID, "error", err)
				continue
			}

			warmer.WarmCache(ctx, ogImage)
			backfilled++
		}

		slog.InfoContext(ctx, "og-image-backfill: completed", "candidates", len(candidates), "backfilled", backfilled)
		return nil
	}
}
