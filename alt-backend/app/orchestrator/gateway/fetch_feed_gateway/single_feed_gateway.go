package fetch_feed_gateway

import (
	"alt/domain"
	"alt/utils"
	"alt/utils/errors"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/mmcdole/gofeed"
)

// PollableFeedLinkStore is the subscription work list (capability catalog
// §2.F W3-F8). Only the reads: the RSS fetch this gateway performs afterwards
// is external HTTP and stays here (ADR-000954 D4).
type PollableFeedLinkStore interface {
	FetchRSSFeedURLs(ctx context.Context) ([]domain.FeedLink, error)
}

type SingleFeedGateway struct {
	store       PollableFeedLinkStore
	rateLimiter *rate_limiter.HostRateLimiter
}

func NewSingleFeedGateway(store PollableFeedLinkStore) *SingleFeedGateway {
	return NewSingleFeedGatewayWithRateLimiter(store, nil)
}

func NewSingleFeedGatewayWithRateLimiter(store PollableFeedLinkStore, rateLimiter *rate_limiter.HostRateLimiter) *SingleFeedGateway {
	if store == nil {
		panic("fetch_feed_gateway: PollableFeedLinkStore is required (see .claude/rules/di-wiring.md)")
	}
	return &SingleFeedGateway{store: store, rateLimiter: rateLimiter}
}

func (g *SingleFeedGateway) FetchSingleFeed(ctx context.Context) (*domain.RSSFeed, error) {
	// Get RSS feed links from the data plane
	feedLinks, err := g.store.FetchRSSFeedURLs(ctx)
	if err != nil {
		dbErr := errors.NewDatabaseUnavailableError(
			"gateway",
			"SingleFeedGateway",
			"FetchRSSFeedURLs",
			err,
			map[string]interface{}{
				"component": "SingleFeedGateway",
				"operation": "fetch_rss_feed_urls",
			},
		)
		logger.SafeLogErrorWithAppContext(ctx, "fetch_rss_feed_urls", dbErr)
		return nil, dbErr
	}

	if len(feedLinks) == 0 {
		logger.SafeLogInfo(ctx, "No RSS feed URLs found in database")
		return &domain.RSSFeed{
			Title:       "No feeds available",
			Description: "No RSS feed URLs have been registered",
			Items:       []domain.FeedItem{},
		}, nil
	}

	// Use the first available feed link
	feedURL, err := url.Parse(feedLinks[0].URL)
	if err != nil {
		return nil, fmt.Errorf("invalid feed link URL: %w", err)
	}
	logger.SafeLogInfo(ctx, "Fetching RSS feed", "url", feedURL.String())

	// Apply rate limiting if rate limiter is configured
	if g.rateLimiter != nil {
		logger.SafeLogInfo(ctx, "Applying rate limiting for external single feed request", "url", feedURL.String())
		if err := g.rateLimiter.WaitForHost(ctx, feedURL.String()); err != nil {
			rateLimitErr := errors.NewRateLimitExceededError(
				"gateway",
				"SingleFeedGateway",
				"WaitForHost",
				err,
				map[string]interface{}{
					"component": "SingleFeedGateway",
					"operation": "rate_limit_wait",
					"url":       feedURL.String(),
					"host":      feedURL.Host,
				},
			)
			logger.SafeLogErrorWithAppContext(ctx, "rate_limit_wait", rateLimitErr)
			return nil, rateLimitErr
		}
		logger.SafeLogInfo(ctx, "Rate limiting passed, proceeding with single feed request", "url", feedURL.String())
	}

	// Parse the RSS feed from the URL using unified HTTP client factory
	factory := utils.NewHTTPClientFactory()
	httpClient := factory.CreateHTTPClient()
	fp := newBoundedFeedParser(httpClient)

	feedCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	feed, err := fp.ParseURLWithContext(feedURL.String(), feedCtx)
	if err != nil {
		apiErr := errors.NewExternalServiceUnavailableError(
			"gateway",
			"SingleFeedGateway",
			"ParseURL",
			err,
			map[string]interface{}{
				"component": "SingleFeedGateway",
				"operation": "external_feed_parse",
				"url":       feedURL.String(),
				"parser":    "gofeed",
			},
		)
		logger.SafeLogErrorWithAppContext(ctx, "external_feed_parse", apiErr)
		return nil, apiErr
	}

	// Convert the gofeed.Feed to domain.RSSFeed
	domainFeed := convertGofeedToDomain(feed)

	logger.SafeLogInfo(ctx, "Successfully fetched RSS feed", "title", domainFeed.Title, "items", len(domainFeed.Items))

	return domainFeed, nil
}

// convertGofeedToDomain converts a gofeed.Feed to domain.RSSFeed
func convertGofeedToDomain(feed *gofeed.Feed) *domain.RSSFeed {
	domainFeed := &domain.RSSFeed{
		Title:       feed.Title,
		Description: feed.Description,
		Link:        feed.Link, // RSS-spec channel link; gofeed uses .Link
		FeedLink:    feed.FeedLink,
		Updated:     feed.Updated,
		Language:    feed.Language,
		Generator:   feed.Generator,
		FeedType:    feed.FeedType,
		FeedVersion: feed.FeedVersion,
		Items:       make([]domain.FeedItem, 0, len(feed.Items)),
	}

	// Handle updated time parsing
	if feed.UpdatedParsed != nil {
		domainFeed.UpdatedParsed = *feed.UpdatedParsed
	}

	// Handle feed image
	if feed.Image != nil {
		domainFeed.Image = domain.RSSFeedImage{
			URL:   feed.Image.URL,
			Title: feed.Image.Title,
		}
	}

	// Handle feed links
	if len(feed.Links) > 0 {
		domainFeed.Links = make([]string, len(feed.Links))
		copy(domainFeed.Links, feed.Links)
	}

	// Convert feed items
	for _, item := range feed.Items {
		domainItem := domain.FeedItem{
			Title:       item.Title,
			Description: item.Description,
			Link:        item.Link,
			Published:   item.Published,
			Links:       item.Links,
		}

		// Handle Author with nil check
		if item.Author != nil {
			domainItem.Author = domain.Author{
				Name: item.Author.Name,
			}
			domainItem.Authors = []domain.Author{
				{
					Name: item.Author.Name,
				},
			}
		}

		// Handle published time parsing
		if item.PublishedParsed != nil {
			domainItem.PublishedParsed = *item.PublishedParsed
		}

		// Handle authors
		if len(item.Authors) > 0 {
			domainItem.Authors = make([]domain.Author, len(item.Authors))
			for i, author := range item.Authors {
				domainItem.Authors[i] = domain.Author{
					Name: author.Name,
				}
			}
			// Set the first author as the main author
			domainItem.Author = domainItem.Authors[0]
		}

		domainFeed.Items = append(domainFeed.Items, domainItem)
	}

	return domainFeed
}
