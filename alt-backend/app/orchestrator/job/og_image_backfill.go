package job

import (
	"alt/domain"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/utils/html_parser"
	"context"
	"fmt"
	"log/slog"
)

// ogBackfillBatchLimit bounds how many article pages are scraped per run, to
// cap external HTTP load. Per-host rate limiting is enforced by the fetcher.
const ogBackfillBatchLimit = 50

// ogBackfillCandidateLister abstracts the work-list query.
//
// Backed by datahub_gateway.OgImageGateway since ADR-000954 Wave 3. The
// element type moved to domain with it: the job runs in a different process
// from the query now, and a driver struct in this signature would put the
// database driver in alt-harvester's import graph for the sake of two strings.
type ogBackfillCandidateLister interface {
	FetchFeedsMissingOgImage(ctx context.Context, limit int) ([]domain.OgImageBackfillCandidate, error)
}

// articleContentFetcher abstracts the SSRF-protected, rate-limited article page
// fetch (for testability).
type articleContentFetcher interface {
	FetchArticleContents(ctx context.Context, articleURL string) (*string, error)
}

// articleHeadSaver abstracts persisting the scraped head + og:image.
//
// Backed by datahub_gateway.OgImageGateway since ADR-000954 Wave 3 batch 2
// (catalog W3-B2), which was the last direct database call this job made. The
// scrape that produces the markup stays here — it is an outbound HTTP fetch,
// and D4 keeps those on the calling side.
type articleHeadSaver interface {
	SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error
}

// OgImageBackfillJob returns a JobScheduler function that scrapes og:image for
// recent feeds that have no image yet, persists it to article_heads, and warms
// the image proxy cache. Coverage complement to the OGP image warmer, which
// only warms feeds that already have an og_image_url.
func OgImageBackfillJob(
	lister ogBackfillCandidateLister,
	saver articleHeadSaver,
	fetcher articleContentFetcher,
	imageProxy *image_proxy_usecase.ImageProxyUsecase,
) func(ctx context.Context) error {
	if lister == nil || saver == nil || fetcher == nil || imageProxy == nil {
		// imageProxy is legitimately nil when IMAGE_PROXY_ENABLED=false or
		// misconfigured (see logImageProxyWiringState in di/image_module.go,
		// finding [6]) — that DI-level log already distinguishes "disabled"
		// from "wiring bug". This per-tick log must stay Warn (not Info) with
		// an explicit disabled reason so it doesn't read as ordinary
		// operational noise to anyone scanning logs for problems.
		return func(ctx context.Context) error {
			slog.WarnContext(ctx, "og-image-backfill skipped: dependencies_disabled")
			return nil
		}
	}
	return ogImageBackfillJobFn(lister, fetcher, saver, imageProxy)
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
