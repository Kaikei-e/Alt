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
	"time"

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
	// SaveFeedOgImage takes the bar already computed rather than deriving it,
	// because this usecase has to report the very same number to the reader's
	// client and two derivations could disagree. retryAfter is 0 for a
	// resolution and for a refusal that is settled inside this window.
	SaveFeedOgImage(ctx context.Context, feedID, ogImageURL string, refusal domain.OgImageRefusal, retryAfter time.Duration) error
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

// Execute resolves what it can and returns two disjoint answers: feed id →
// proxy URL for what resolved, and feed id → seconds-before-asking-again for
// what did not.
//
// The two lists, and the third outcome carried by absence from both, are the
// contract documented on ResolveOgImagesResponse in
// proto/alt/feeds/v2/feeds.proto. Restated in the terms this code works in:
//
//	resolved                 — an image is held or was just obtained.
//	unresolved, seconds > 0  — the origin was asked and failed. The bar is real
//	                           and the client may come back after it.
//	unresolved, seconds == 0 — the origin was asked and gave a settled answer,
//	                           or there is no page to ask. Nothing to come back
//	                           for inside this retention window.
//	neither list             — no origin was spent: the batch cap trimmed the
//	                           feed, no row exists for it, or the fetch failed
//	                           on our side. The client may ask again at once.
//
// A feed that could not be resolved is absent from `resolved` rather than
// mapped to an empty string, so the caller cannot render "we were refused" as
// "we hold a blank image".
func (u *Usecase) Execute(ctx context.Context, feedIDs []string) (map[string]string, map[string]int64, error) {
	if len(feedIDs) == 0 {
		return map[string]string{}, map[string]int64{}, nil
	}
	if len(feedIDs) > MaxBatch {
		// The feeds beyond the cap are reported in neither list. Nothing was
		// spent on them and nothing is known about them, and inventing a bar
		// for a feed we declined to look at would hold a card back for a
		// decision that was ours.
		feedIDs = feedIDs[:MaxBatch]
	}

	targets, err := u.store.FetchFeedOgImageTargets(ctx, feedIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve og images: %w", err)
	}

	resolved := make(map[string]string, len(targets))
	unresolved := make(map[string]int64, len(targets))
	for _, target := range targets {
		// Already held: mint and move on. No request leaves this process.
		if target.OgImageURL != "" {
			resolved[target.FeedID] = u.minter.GenerateProxyURL(target.OgImageURL)
			continue
		}
		// Already refused, or nothing to fetch: stay away from the origin, and
		// hand on whatever is left of the bar an earlier attempt set.
		//
		// This is the branch every failing feed takes from its second ask
		// onwards, so reporting a flat zero here would be the thing that makes
		// escalation invisible: a feed held back for five seconds and a feed
		// refused by robots.txt would look identical to the client, and it
		// would give both cards up for the session.
		//
		// A feed whose page URL is empty lands here too, with a zero — correct
		// for the same reason as a settled refusal. feeds.website_url will not
		// change inside this window, so there is nothing to come back for, and
		// saying so beats the silence that would have the client re-ask on
		// every page load forever.
		if !target.NeedsFetch() {
			unresolved[target.FeedID] = target.RetryAfterSeconds
			continue
		}

		// attempts counts the attempt about to be made, not the ones behind it.
		// The stored counter is 0 for a feed with no row and n after n attempts,
		// so +1 names this one — and it is the same number the driver's upsert
		// will land on, which keeps the bar written into the row equal to the
		// bar handed to the client.
		attempts := target.Attempts + 1

		image, refusal, fetchErr := u.fetcher.FetchOgImage(ctx, target.PageURL)
		if fetchErr != nil {
			// A malformed URL or an SSRF rejection is our problem, not the
			// origin's answer, so it is not recorded as a refusal — that would
			// suppress a feed for a fault on our side. For the same reason it
			// is reported in neither list: both lists say what the origin
			// answered, and we never managed to ask.
			slog.WarnContext(ctx, "og-image-resolve: fetch failed",
				"feed_id", target.FeedID, "error", fetchErr)
			continue
		}

		if refusal != "" {
			retryAfter := refusal.RetryAfter(attempts)
			if err := u.store.SaveFeedOgImage(ctx, target.FeedID, "", refusal, retryAfter); err != nil {
				slog.WarnContext(ctx, "og-image-resolve: could not record refusal",
					"feed_id", target.FeedID, "reason", string(refusal), "error", err)
			}
			// Reported whether or not the write landed. The bar is the honest
			// answer to what just happened; a failed write costs the next
			// reader an extra origin request, but withholding it here would
			// cost this reader the card.
			unresolved[target.FeedID] = int64(retryAfter.Seconds())
			continue
		}

		if err := u.store.SaveFeedOgImage(ctx, target.FeedID, image, "", 0); err != nil {
			// The image is good even if we failed to remember it; serve it now
			// and let the next viewport entry re-resolve.
			slog.WarnContext(ctx, "og-image-resolve: could not record resolution",
				"feed_id", target.FeedID, "error", err)
		}

		// The browser is about to ask the proxy for exactly this URL.
		u.minter.WarmCache(ctx, image)
		resolved[target.FeedID] = u.minter.GenerateProxyURL(image)
	}

	return resolved, unresolved, nil
}
