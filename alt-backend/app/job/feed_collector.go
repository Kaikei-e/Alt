package job

import (
	"alt/domain"
	"alt/port/feed_link_availability_port"
	"alt/utils"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	rssFeed "github.com/mmcdole/gofeed"
)

const maxConsecutiveFailures = 5

func CollectSingleFeed(ctx context.Context, feedURL url.URL, rateLimiter *rate_limiter.HostRateLimiter) (*rssFeed.Feed, error) {
	// Apply rate limiting if rate limiter is configured
	if rateLimiter != nil {
		slog.InfoContext(ctx, "Applying rate limiting for single feed collection", "url", feedURL.String())
		if err := rateLimiter.WaitForHost(ctx, feedURL.String()); err != nil {
			slog.ErrorContext(ctx, "Rate limiting failed for single feed collection", "url", feedURL.String(), "error", err)
			return nil, fmt.Errorf("rate limiting failed: %w", err)
		}
		slog.InfoContext(ctx, "Rate limiting passed, proceeding with single feed collection", "url", feedURL.String())
	}

	// Use unified HTTP client factory for secure RSS feed fetching
	factory := utils.NewHTTPClientFactory()
	httpClient := factory.CreateHTTPClient()
	fp := rssFeed.NewParser()
	fp.Client = httpClient
	feed, err := fp.ParseURL(feedURL.String())
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Error parsing feed", "error", err)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "Feed collected", "feed title", feed.Title)

	return feed, nil
}

// validateFeedURL performs basic scheme/host validation on a feed URL.
// Network reachability is verified by gofeed.ParseURL (GET request) in the collection loop.
func validateFeedURL(_ context.Context, feedURL url.URL) error {
	if feedURL.Scheme != "http" && feedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (must be http or https)", feedURL.Scheme)
	}
	if feedURL.Host == "" {
		return fmt.Errorf("missing host in URL")
	}
	return nil
}

func CollectMultipleFeeds(ctx context.Context, feedLinks []domain.FeedLink, rateLimiter *rate_limiter.HostRateLimiter, availabilityRepo feed_link_availability_port.FeedLinkAvailabilityPort) ([]*domain.FeedItem, error) {
	// Use unified HTTP client factory for secure RSS feed fetching
	factory := utils.NewHTTPClientFactory()
	httpClient := factory.CreateHTTPClient()
	fp := rssFeed.NewParser()
	fp.Client = httpClient
	var feedItems []*domain.FeedItem
	var errors []error

	for i, feedLink := range feedLinks {
		feedURL, err := url.Parse(feedLink.URL)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Invalid feed link URL", "url", feedLink.URL, "error", err)
			errors = append(errors, err)
			continue
		}

		// First validate the URL
		if err := validateFeedURL(ctx, *feedURL); err != nil {
			logger.Logger.ErrorContext(ctx, "Feed URL validation failed", "url", feedURL.String(), "error", err)
			errors = append(errors, err)
			handleFeedError(ctx, *feedURL, err, rateLimiter, availabilityRepo)
			continue
		}

		// Apply rate limiting before parsing
		if rateLimiter != nil {
			slog.InfoContext(ctx, "Applying rate limiting for multiple feed collection", "url", feedURL.String())
			if err := rateLimiter.WaitForHost(ctx, feedURL.String()); err != nil {
				slog.ErrorContext(ctx, "Rate limiting failed for multiple feed collection", "url", feedURL.String(), "error", err)
				errors = append(errors, fmt.Errorf("rate limiting failed: %w", err))
				continue
			}
			slog.InfoContext(ctx, "Rate limiting passed, proceeding with multiple feed collection", "url", feedURL.String())
		}

		feed, err := fetchWithRetryOn403(ctx, func() (*rssFeed.Feed, error) {
			return fp.ParseURL(feedURL.String())
		}, feedURL.String())
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error parsing feed", "url", feedURL.String(), "error", err)
			errors = append(errors, err)
			handleFeedError(ctx, *feedURL, err, rateLimiter, availabilityRepo)
			continue
		}

		logger.Logger.InfoContext(ctx, "Successfully parsed feed", "url", feedURL.String(), "title", feed.Title)

		// Convert feed items and set feed_link_id
		items := ConvertFeedToFeedItem([]*rssFeed.Feed{feed})
		feedLinkIDStr := feedLink.ID.String()
		for _, item := range items {
			item.FeedLinkID = &feedLinkIDStr
		}
		feedItems = append(feedItems, items...)

		// Reset failure count on success
		if availabilityRepo != nil {
			if err := availabilityRepo.ResetFeedLinkFailures(ctx, feedURL.String()); err != nil {
				logger.Logger.WarnContext(ctx, "Failed to reset feed failures", "url", feedURL.String(), "error", err)
			}
		}

		// Note: Rate limiting replaced the hardcoded sleep
		logger.Logger.InfoContext(ctx, "Feed collection progress", "current", i+1, "total", len(feedLinks))
	}

	// Log summary of collection results
	successCount := len(feedLinks) - len(errors)
	logger.Logger.InfoContext(ctx, "Feed collection summary", "successful", successCount, "failed", len(errors), "total", len(feedLinks))

	// Only return error if all feeds failed
	if len(feedItems) == 0 && len(errors) > 0 {
		logger.Logger.ErrorContext(ctx, "All feeds failed to parse", "total_errors", len(errors))
		return nil, errors[0] // Return the first error
	}

	logger.Logger.InfoContext(ctx, "Feed items", "feedItems count", len(feedItems))
	return feedItems, nil
}

// handleFeedError categorizes the error and takes appropriate action.
func handleFeedError(ctx context.Context, feedURL url.URL, err error, rateLimiter *rate_limiter.HostRateLimiter, availabilityRepo feed_link_availability_port.FeedLinkAvailabilityPort) {
	if is429Error(err) {
		// For rate limiting errors, increase backoff for this host
		if rateLimiter != nil {
			rateLimiter.RecordRateLimitHit(feedURL.Host, 0) // 0 means use default backoff
			logger.Logger.WarnContext(ctx, "Rate limited by target site, backing off",
				"url", feedURL.String())
		}
	} else if isPersistentError(err) && availabilityRepo != nil {
		// For persistent errors, increment failure count
		availability, dbErr := availabilityRepo.IncrementFeedLinkFailures(ctx, feedURL.String(), err.Error())
		if dbErr != nil {
			logger.Logger.ErrorContext(ctx, "Failed to track feed failure", "url", feedURL.String(), "error", dbErr)
			return
		}

		// Use domain business logic to determine if feed should be disabled
		if availability.ShouldDisable(maxConsecutiveFailures) {
			if disableErr := availabilityRepo.DisableFeedLink(ctx, feedURL.String()); disableErr != nil {
				logger.Logger.ErrorContext(ctx, "Failed to disable feed", "url", feedURL.String(), "error", disableErr)
			} else {
				logger.Logger.WarnContext(ctx, "Auto-disabled feed after repeated failures",
					"url", feedURL.String(),
					"failures", availability.ConsecutiveFailures)
			}
		}
	}
}

// isPersistentError returns true for errors that indicate persistent issues with the feed.
// Note: 403 is included because fetchWithRetryOn403 exhausts retries before this is called.
func isPersistentError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "400") ||
		strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "404") ||
		strings.Contains(errStr, "Failed to detect feed type")
}

// is403Error returns true if the error indicates a 403 Forbidden response.
func is403Error(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "403")
}

const max403Retries = 3

// fetchWithRetryOn403 retries the fetch with exponential backoff when a 403 is received.
// After max403Retries, the 403 error is returned and treated as persistent by the caller.
func fetchWithRetryOn403(ctx context.Context, fetchFn func() (*rssFeed.Feed, error), feedURL string) (*rssFeed.Feed, error) {
	feed, err := fetchFn()
	if err == nil || !is403Error(err) {
		return feed, err
	}

	for attempt := 1; attempt <= max403Retries; attempt++ {
		backoff := time.Duration(1<<uint(attempt-1)) * time.Second // 1s, 2s, 4s
		slog.WarnContext(ctx, "403 received, retrying with exponential backoff",
			"url", feedURL, "attempt", attempt, "backoff", backoff)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		feed, err = fetchFn()
		if err == nil || !is403Error(err) {
			return feed, err
		}
	}

	return nil, err
}

// is429Error returns true if the error indicates rate limiting by the target site.
func is429Error(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "429")
}

func ConvertFeedToFeedItem(feeds []*rssFeed.Feed) []*domain.FeedItem {
	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		for _, item := range feed.Items {
			// Skip items with empty or invalid titles
			if strings.TrimSpace(item.Title) == "" {
				logger.SafeWarn("Skipping feed item with empty title",
					"link", item.Link,
					"description", truncateString(item.Description, 100))
				continue
			}

			// Skip items with suspicious content that indicates 404 errors
			if strings.Contains(strings.ToLower(item.Description), "404 page not found") ||
				strings.Contains(strings.ToLower(item.Title), "404") ||
				strings.Contains(strings.ToLower(item.Title), "not found") {
				logger.SafeWarn("Skipping feed item with 404/not found content",
					"title", item.Title,
					"link", item.Link)
				continue
			}

			var author domain.Author
			var authors []domain.Author
			if item.Author != nil {
				author = domain.Author{Name: item.Author.Name}
				authors = append(authors, author)
			}

			// Handle nil PublishedParsed to avoid nil pointer dereference
			var publishedParsed time.Time
			if item.PublishedParsed != nil {
				publishedParsed = *item.PublishedParsed
			} else {
				// Use zero time if PublishedParsed is nil (will be handled in job_runner.go)
				publishedParsed = time.Time{}
			}

			feedItems = append(feedItems, &domain.FeedItem{
				Title:           strings.TrimSpace(item.Title),
				Description:     item.Description,
				Link:            item.Link,
				PublishedParsed: publishedParsed,
				Author:          author,
				Authors:         authors,
				OgImageURL:      ExtractImageURL(item),
			})
		}
	}
	return feedItems
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}
