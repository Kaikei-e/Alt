package fetch_feed_gateway

import (
	"alt/domain"
	"alt/utils"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
	"alt/utils/sanitize"
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/mmcdole/gofeed"
)

// maxFeedBodyBytes bounds how much of a feed body gofeed reads. gofeed treats a
// zero MaxByteSize as "no limit", and the transport transparently decompresses
// gzip, so without a ceiling a few KB on the wire can expand into GBs resident.
const maxFeedBodyBytes = 10 * 1024 * 1024

// newBoundedFeedParser builds a gofeed parser that refuses bodies larger than
// maxFeedBodyBytes instead of reading them in full.
func newBoundedFeedParser(client *http.Client) *gofeed.Parser {
	fp := gofeed.NewParser()
	fp.Client = client
	fp.MaxByteSize = maxFeedBodyBytes
	fp.UserAgent = "Alt-RSS-Reader/1.0 (+https://alt.example.com)"
	return fp
}

type FetchFeedsGateway struct {
	store       FeedListStore
	rateLimiter *rate_limiter.HostRateLimiter
	httpClient  *http.Client
}

func NewFetchFeedsGateway(store FeedListStore) *FetchFeedsGateway {
	return NewFetchFeedsGatewayWithRateLimiter(store, nil)
}

func NewFetchFeedsGatewayWithRateLimiter(store FeedListStore, rateLimiter *rate_limiter.HostRateLimiter) *FetchFeedsGateway {
	if store == nil {
		panic("fetch_feed_gateway: FeedListStore is required — a nil one would make every feed list " +
			"fail identically to a user with no subscriptions (see .claude/rules/di-wiring.md)")
	}
	return &FetchFeedsGateway{
		store:       store,
		rateLimiter: rateLimiter,
		httpClient:  nil,
	}
}

func (g *FetchFeedsGateway) FetchFeeds(ctx context.Context, link string) ([]*domain.FeedItem, error) {
	// Apply rate limiting if rate limiter is configured
	if g.rateLimiter != nil {
		slog.InfoContext(ctx, "Applying rate limiting for external feed request", "url", link)
		if err := g.rateLimiter.WaitForHost(ctx, link); err != nil {
			slog.ErrorContext(ctx, "Rate limiting failed", "url", link, "error", err)
			return nil, errors.New("rate limiting failed")
		}
		slog.InfoContext(ctx, "Rate limiting passed, proceeding with feed request", "url", link)
	}

	// Use provided HTTP client if available, otherwise create a secure one
	httpClient := g.httpClient
	if httpClient == nil {
		factory := utils.NewHTTPClientFactory()
		httpClient = factory.CreateHTTPClient()
	}

	fp := newBoundedFeedParser(httpClient)
	feed, err := fp.ParseURL(link)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error parsing feed", "error", err)
		return nil, errors.New("error parsing feed")
	}

	var feedItems []*domain.FeedItem
	for _, item := range feed.Items {
		feedItem := &domain.FeedItem{
			Title:       item.Title,
			Description: sanitize.SanitizeDescription(item.Description),
			Link:        item.Link,
			Published:   item.Published,
			Links:       item.Links,
		}

		// Handle PublishedParsed with nil check
		if item.PublishedParsed != nil {
			feedItem.PublishedParsed = *item.PublishedParsed
		}

		// Handle Author with nil check
		if item.Author != nil {
			feedItem.Author = domain.Author{
				Name: item.Author.Name,
			}
			feedItem.Authors = []domain.Author{
				{
					Name: item.Author.Name,
				},
			}
		}

		feedItems = append(feedItems, feedItem)
	}

	return feedItems, nil
}
