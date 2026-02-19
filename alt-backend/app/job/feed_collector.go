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

// validateFeedURL performs basic validation on a feed URL before attempting to parse it
func validateFeedURL(ctx context.Context, feedURL url.URL) error {
	// Check if URL scheme is valid
	if feedURL.Scheme != "http" && feedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (must be http or https)", feedURL.Scheme)
	}

	// Check if host is present
	if feedURL.Host == "" {
		return fmt.Errorf("missing host in URL")
	}

	// Try to make a HEAD request to check if URL is accessible using unified HTTP client factory
	factory := utils.NewHTTPClientFactory()
	client := factory.CreateHTTPClient()

	resp, err := client.Head(feedURL.String())
	if err != nil {
		return fmt.Errorf("failed to access URL: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Logger.DebugContext(ctx, "Failed to close response body", "error", closeErr)
		}
	}()

	// Log response details for debugging
	logger.Logger.InfoContext(ctx, "Feed URL validation response",
		"url", feedURL.String(),
		"status_code", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
		"content_length", resp.Header.Get("Content-Length"))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	// Check if content type suggests this might be an RSS/XML feed
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "xml") && !strings.Contains(contentType, "rss") && !strings.Contains(contentType, "atom") {
		logger.SafeWarnContext(ctx, "Content type may not be RSS/XML",
			"url", feedURL.String(),
			"content_type", contentType)
	}

	return nil
}

func CollectMultipleFeeds(ctx context.Context, feedURLs []url.URL, rateLimiter *rate_limiter.HostRateLimiter, availabilityRepo feed_link_availability_port.FeedLinkAvailabilityPort) ([]*domain.FeedItem, error) {
	// Use unified HTTP client factory for secure RSS feed fetching
	factory := utils.NewHTTPClientFactory()
	httpClient := factory.CreateHTTPClient()
	fp := rssFeed.NewParser()
	fp.Client = httpClient
	var feeds []*rssFeed.Feed
	var errors []error

	for i, feedURL := range feedURLs {
		// First validate the URL
		if err := validateFeedURL(ctx, feedURL); err != nil {
			logger.Logger.ErrorContext(ctx, "Feed URL validation failed", "url", feedURL.String(), "error", err)
			errors = append(errors, err)
			handleFeedError(ctx, feedURL, err, rateLimiter, availabilityRepo)
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

		feed, err := fp.ParseURL(feedURL.String())
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error parsing feed", "url", feedURL.String(), "error", err)
			errors = append(errors, err)
			handleFeedError(ctx, feedURL, err, rateLimiter, availabilityRepo)
			continue // Continue processing other feeds instead of failing entirely
		}

		feeds = append(feeds, feed)
		logger.Logger.InfoContext(ctx, "Successfully parsed feed", "url", feedURL.String(), "title", feed.Title)

		// Reset failure count on success
		if availabilityRepo != nil {
			if err := availabilityRepo.ResetFeedLinkFailures(ctx, feedURL.String()); err != nil {
				logger.Logger.WarnContext(ctx, "Failed to reset feed failures", "url", feedURL.String(), "error", err)
			}
		}

		// Note: Rate limiting replaced the hardcoded sleep
		logger.Logger.InfoContext(ctx, "Feed collection progress", "current", i+1, "total", len(feedURLs))
	}

	// Log summary of collection results
	logger.Logger.InfoContext(ctx, "Feed collection summary", "successful", len(feeds), "failed", len(errors), "total", len(feedURLs))

	// Only return error if all feeds failed
	if len(feeds) == 0 && len(errors) > 0 {
		logger.Logger.ErrorContext(ctx, "All feeds failed to parse", "total_errors", len(errors))
		return nil, errors[0] // Return the first error
	}

	feedItems := ConvertFeedToFeedItem(feeds)
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
