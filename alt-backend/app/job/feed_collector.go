package job

import (
	"alt/domain"
	"alt/utils"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	rssFeed "github.com/mmcdole/gofeed"
)

func CollectSingleFeed(ctx context.Context, feedURL url.URL, rateLimiter *rate_limiter.HostRateLimiter) (*rssFeed.Feed, error) {
	// Apply rate limiting if rate limiter is configured
	if rateLimiter != nil {
		slog.Info("Applying rate limiting for single feed collection", "url", feedURL.String())
		if err := rateLimiter.WaitForHost(ctx, feedURL.String()); err != nil {
			slog.Error("Rate limiting failed for single feed collection", "url", feedURL.String(), "error", err)
			return nil, fmt.Errorf("rate limiting failed: %w", err)
		}
		slog.Info("Rate limiting passed, proceeding with single feed collection", "url", feedURL.String())
	}

	// Use unified HTTP client factory for secure RSS feed fetching
	factory := utils.NewHTTPClientFactory()
	httpClient := factory.CreateHTTPClient()
	fp := rssFeed.NewParser()
	fp.Client = httpClient
	feed, err := fp.ParseURL(feedURL.String())
	if err != nil {
		logger.Logger.Error("Error parsing feed", "error", err)
		return nil, err
	}

	logger.Logger.Info("Feed collected", "feed title", feed.Title)

	return feed, nil
}

// validateFeedURL performs basic validation on a feed URL before attempting to parse it
func validateFeedURL(ctx context.Context, feedURL url.URL, rateLimiter *rate_limiter.HostRateLimiter) error {
	// Check if URL scheme is valid
	if feedURL.Scheme != "http" && feedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (must be http or https)", feedURL.Scheme)
	}

	// Check if host is present
	if feedURL.Host == "" {
		return fmt.Errorf("missing host in URL")
	}

	// Apply rate limiting if rate limiter is configured
	if rateLimiter != nil {
		slog.Info("Applying rate limiting for feed URL validation", "url", feedURL.String())
		if err := rateLimiter.WaitForHost(ctx, feedURL.String()); err != nil {
			slog.Error("Rate limiting failed for feed URL validation", "url", feedURL.String(), "error", err)
			return fmt.Errorf("rate limiting failed for validation: %w", err)
		}
		slog.Info("Rate limiting passed, proceeding with feed URL validation", "url", feedURL.String())
	}

	// Try to make a HEAD request to check if URL is accessible using unified HTTP client factory
	factory := utils.NewHTTPClientFactory()
	client := factory.CreateHTTPClient()

	resp, err := client.Head(feedURL.String())
	if err != nil {
		return fmt.Errorf("failed to access URL: %w", err)
	}
	defer resp.Body.Close()

	// Log response details for debugging
	logger.Logger.Info("Feed URL validation response",
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
		logger.Logger.Warn("Content type may not be RSS/XML",
			"url", feedURL.String(),
			"content_type", contentType)
	}

	return nil
}

func CollectMultipleFeeds(ctx context.Context, feedURLs []url.URL, rateLimiter *rate_limiter.HostRateLimiter) ([]*domain.FeedItem, error) {
	// Use unified HTTP client factory for secure RSS feed fetching
	factory := utils.NewHTTPClientFactory()
	httpClient := factory.CreateHTTPClient()
	fp := rssFeed.NewParser()
	fp.Client = httpClient
	var feeds []*rssFeed.Feed
	var errors []error

	for i, feedURL := range feedURLs {
		// First validate the URL
		if err := validateFeedURL(ctx, feedURL, rateLimiter); err != nil {
			logger.Logger.Error("Feed URL validation failed", "url", feedURL.String(), "error", err)
			errors = append(errors, err)
			continue
		}

		// Apply rate limiting before parsing (separate from validation rate limiting)
		if rateLimiter != nil {
			slog.Info("Applying rate limiting for multiple feed collection", "url", feedURL.String())
			if err := rateLimiter.WaitForHost(ctx, feedURL.String()); err != nil {
				slog.Error("Rate limiting failed for multiple feed collection", "url", feedURL.String(), "error", err)
				errors = append(errors, fmt.Errorf("rate limiting failed: %w", err))
				continue
			}
			slog.Info("Rate limiting passed, proceeding with multiple feed collection", "url", feedURL.String())
		}

		feed, err := fp.ParseURL(feedURL.String())
		if err != nil {
			logger.Logger.Error("Error parsing feed", "url", feedURL.String(), "error", err)
			errors = append(errors, err)
			continue // Continue processing other feeds instead of failing entirely
		}

		feeds = append(feeds, feed)
		logger.Logger.Info("Successfully parsed feed", "url", feedURL.String(), "title", feed.Title)

		// Note: Rate limiting replaced the hardcoded sleep
		logger.Logger.Info("Feed collection progress", "current", i+1, "total", len(feedURLs))
	}

	// Log summary of collection results
	logger.Logger.Info("Feed collection summary", "successful", len(feeds), "failed", len(errors), "total", len(feedURLs))

	// Only return error if all feeds failed
	if len(feeds) == 0 && len(errors) > 0 {
		logger.Logger.Error("All feeds failed to parse", "total_errors", len(errors))
		return nil, errors[0] // Return the first error
	}

	feedItems := ConvertFeedToFeedItem(feeds)
	logger.Logger.Info("Feed items", "feedItems count", len(feedItems))
	return feedItems, nil
}

func ConvertFeedToFeedItem(feeds []*rssFeed.Feed) []*domain.FeedItem {
	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		for _, item := range feed.Items {
			// Skip items with empty or invalid titles
			if strings.TrimSpace(item.Title) == "" {
				logger.Logger.Warn("Skipping feed item with empty title", 
					"link", item.Link, 
					"description", truncateString(item.Description, 100))
				continue
			}

			// Skip items with suspicious content that indicates 404 errors
			if strings.Contains(strings.ToLower(item.Description), "404 page not found") ||
				strings.Contains(strings.ToLower(item.Title), "404") ||
				strings.Contains(strings.ToLower(item.Title), "not found") {
				logger.Logger.Warn("Skipping feed item with 404/not found content", 
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
			feedItems = append(feedItems, &domain.FeedItem{
				Title:           strings.TrimSpace(item.Title),
				Description:     item.Description,
				Link:            item.Link,
				PublishedParsed: *item.PublishedParsed,
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

