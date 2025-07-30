package register_feed_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mmcdole/gofeed"
)

// RSSFeedFetcher interface for mocking RSS feed fetching
type RSSFeedFetcher interface {
	FetchRSSFeed(ctx context.Context, link string) (*gofeed.Feed, error)
}

// DefaultRSSFeedFetcher implements RSSFeedFetcher with actual HTTP requests
type DefaultRSSFeedFetcher struct{}

func (f *DefaultRSSFeedFetcher) FetchRSSFeed(ctx context.Context, link string) (*gofeed.Feed, error) {
	return f.fetchRSSFeedWithRetry(ctx, link)
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504")
}

// fetchRSSFeedWithRetry performs RSS feed fetching with exponential backoff retry
func (f *DefaultRSSFeedFetcher) fetchRSSFeedWithRetry(ctx context.Context, link string) (*gofeed.Feed, error) {
	const maxRetries = 3
	const initialDelay = 2 * time.Second
	const maxDelay = 30 * time.Second

	// Create HTTP client with extended 45 second timeout
	httpClient := &http.Client{
		Timeout: 45 * time.Second,
	}

	fp := gofeed.NewParser()
	fp.Client = httpClient

	// Use context with extended 60 second timeout for additional protection
	feedCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := time.Duration(float64(initialDelay) * math.Pow(2, float64(attempt-1)))
			if delay > maxDelay {
				delay = maxDelay
			}

			logger.SafeInfo("Retrying RSS feed fetch",
				"url", link,
				"attempt", attempt+1,
				"delay_seconds", delay.Seconds())

			select {
			case <-time.After(delay):
			case <-feedCtx.Done():
				return nil, feedCtx.Err()
			}
		}

		feed, err := fp.ParseURLWithContext(link, feedCtx)
		if err == nil {
			if attempt > 0 {
				logger.SafeInfo("RSS feed fetch succeeded after retry",
					"url", link,
					"attempts", attempt+1)
			}
			return feed, nil
		}

		lastErr = err
		if !isRetryableError(err) {
			logger.SafeWarn("Non-retryable error, not retrying",
				"url", link,
				"error", err.Error())
			break
		}

		logger.SafeWarn("RSS feed fetch failed, will retry",
			"url", link,
			"attempt", attempt+1,
			"error", err.Error())
	}

	return nil, lastErr
}

type RegisterFeedGateway struct {
	alt_db      *alt_db.AltDBRepository
	feedFetcher RSSFeedFetcher
}

func NewRegisterFeedLinkGateway(pool *pgxpool.Pool) *RegisterFeedGateway {
	return &RegisterFeedGateway{
		alt_db:      alt_db.NewAltDBRepositoryWithPool(pool),
		feedFetcher: &DefaultRSSFeedFetcher{},
	}
}

// NewRegisterFeedLinkGatewayWithFetcher creates a gateway with a custom RSS feed fetcher (for testing)
func NewRegisterFeedLinkGatewayWithFetcher(pool *pgxpool.Pool, fetcher RSSFeedFetcher) *RegisterFeedGateway {
	return &RegisterFeedGateway{
		alt_db:      alt_db.NewAltDBRepositoryWithPool(pool),
		feedFetcher: fetcher,
	}
}

func (g *RegisterFeedGateway) RegisterRSSFeedLink(ctx context.Context, link string) error {
	// Parse and validate the URL
	parsedURL, err := url.Parse(link)
	if err != nil {
		return errors.New("invalid URL format")
	}

	// Ensure the URL has a scheme
	if parsedURL.Scheme == "" {
		return errors.New("URL must include a scheme (http or https)")
	}

	// Try to fetch and parse the RSS feed with retry mechanism
	feed, err := g.feedFetcher.FetchRSSFeed(ctx, link)
	if err != nil {
		if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "connection refused") {
			return errors.New("could not reach the RSS feed URL")
		}
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			return errors.New("RSS feed fetch timeout - server took too long to respond")
		}
		return errors.New("invalid RSS feed format")
	}

	if feed.Link == "" {
		logger.SafeWarn("RSS feed link is empty, using the link from the RSS feed", "link", link)
		feed.Link = link
	}

	if feed.FeedLink == "" {
		logger.SafeWarn("RSS feed feed link is empty, using the link from the RSS feed", "link", feed.Link)
		feed.FeedLink = link
	}

	// Check database connection only after RSS feed validation
	if g.alt_db == nil {
		return errors.New("database connection not available")
	}

	err = g.alt_db.RegisterRSSFeedLink(ctx, feed.FeedLink)
	if err != nil {
		if errors.Is(err, pgx.ErrTxClosed) {
			logger.SafeError("Failed to register RSS feed link", "error", err)
			return errors.New("failed to register RSS feed link")
		}
		logger.SafeError("Error registering RSS feed link", "error", err)
		return errors.New("failed to register RSS feed link")
	}
	logger.SafeInfo("RSS feed link registered", "link", link)

	return nil
}
