// Package og_image_resolve_usecase resolves og:image URLs for feeds a reader
// has brought into view.
//
// The shape of this usecase is the ethical argument for it. Every origin
// request it makes is caused by a person looking at that card, it makes at most
// one per feed, and it records refusals so that looking again does not ask
// again. What it replaces was a scheduled sweep with no memory of having been
// told no.
package og_image_resolve_usecase

import (
	"context"
	"fmt"
	"log/slog"

	"alt/domain"
)

// MaxBatch bounds how many feeds one call may resolve.
//
// It is not a pagination knob. It bounds how many publishers a single viewport
// change can cause us to contact, which is the number that matters to them.
const MaxBatch = 10

// FeedOgImageStore is what alt-data-hub knows about a feed's image: whether one
// is already held, and whether the origin has already refused.
type FeedOgImageStore interface {
	FetchFeedOgImageTargets(ctx context.Context, feedIDs []string) ([]domain.FeedOgImageTarget, error)
	SaveFeedOgImage(ctx context.Context, feedID, ogImageURL string, refusal domain.OgImageRefusal) error
}

// OgImageFetcher reads one page's og:image, honouring robots.txt.
type OgImageFetcher interface {
	FetchOgImage(ctx context.Context, pageURL string) (string, domain.OgImageRefusal, error)
}

// ProxyURLMinter signs the image URL the browser will actually request, and can
// pull the bytes into the proxy cache ahead of that request.
type ProxyURLMinter interface {
	GenerateProxyURL(imageURL string) string
	WarmCache(ctx context.Context, imageURL string)
}

type Usecase struct {
	store   FeedOgImageStore
	fetcher OgImageFetcher
	minter  ProxyURLMinter
}

func NewUsecase(store FeedOgImageStore, fetcher OgImageFetcher, minter ProxyURLMinter) *Usecase {
	switch {
	case store == nil:
		panic("og_image_resolve_usecase: a feed og image store is required (see .claude/rules/di-wiring.md)")
	case fetcher == nil:
		panic("og_image_resolve_usecase: an og image fetcher is required — without it resolution would silently return nothing and read as 'no feed has an image'")
	case minter == nil:
		panic("og_image_resolve_usecase: a proxy URL minter is required — a resolved image with no signed URL is unreachable by the browser")
	}
	return &Usecase{store: store, fetcher: fetcher, minter: minter}
}

// Execute resolves what it can and returns feed id → proxy URL.
//
// Feeds that could not be resolved are absent from the map rather than mapped
// to an empty string, so the caller cannot render "we were refused" as "we hold
// a blank image".
func (u *Usecase) Execute(ctx context.Context, feedIDs []string) (map[string]string, error) {
	if len(feedIDs) == 0 {
		return map[string]string{}, nil
	}
	if len(feedIDs) > MaxBatch {
		feedIDs = feedIDs[:MaxBatch]
	}

	targets, err := u.store.FetchFeedOgImageTargets(ctx, feedIDs)
	if err != nil {
		return nil, fmt.Errorf("resolve og images: %w", err)
	}

	resolved := make(map[string]string, len(targets))
	for _, target := range targets {
		// Already held: mint and move on. No request leaves this process.
		if target.OgImageURL != "" {
			resolved[target.FeedID] = u.minter.GenerateProxyURL(target.OgImageURL)
			continue
		}
		// Already refused, or nothing to fetch: stay away from the origin.
		if !target.NeedsFetch() {
			continue
		}

		image, refusal, fetchErr := u.fetcher.FetchOgImage(ctx, target.PageURL)
		if fetchErr != nil {
			// A malformed URL or an SSRF rejection is our problem, not the
			// origin's answer, so it is not recorded as a refusal — that would
			// suppress a feed for a fault on our side.
			slog.WarnContext(ctx, "og-image-resolve: fetch failed",
				"feed_id", target.FeedID, "error", fetchErr)
			continue
		}

		if refusal != "" {
			if err := u.store.SaveFeedOgImage(ctx, target.FeedID, "", refusal); err != nil {
				slog.WarnContext(ctx, "og-image-resolve: could not record refusal",
					"feed_id", target.FeedID, "reason", string(refusal), "error", err)
			}
			continue
		}

		if err := u.store.SaveFeedOgImage(ctx, target.FeedID, image, ""); err != nil {
			// The image is good even if we failed to remember it; serve it now
			// and let the next viewport entry re-resolve.
			slog.WarnContext(ctx, "og-image-resolve: could not record resolution",
				"feed_id", target.FeedID, "error", err)
		}

		// The browser is about to ask the proxy for exactly this URL.
		u.minter.WarmCache(ctx, image)
		resolved[target.FeedID] = u.minter.GenerateProxyURL(image)
	}

	return resolved, nil
}
