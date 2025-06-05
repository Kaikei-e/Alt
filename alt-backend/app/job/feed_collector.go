package job

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	rssFeed "github.com/mmcdole/gofeed"
)

func CollectSingleFeed(ctx context.Context, feedURL url.URL) (*rssFeed.Feed, error) {
	fp := rssFeed.NewParser()
	feed, err := fp.ParseURL(feedURL.String())
	if err != nil {
		logger.Logger.Error("Error parsing feed", "error", err)
		return nil, err
	}

	logger.Logger.Info("Feed collected", "feed title", feed.Title)

	return feed, nil

}

// validateFeedURL performs basic validation on a feed URL before attempting to parse it
func validateFeedURL(feedURL url.URL) error {
	// Check if URL scheme is valid
	if feedURL.Scheme != "http" && feedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (must be http or https)", feedURL.Scheme)
	}

	// Check if host is present
	if feedURL.Host == "" {
		return fmt.Errorf("missing host in URL")
	}

	// Try to make a HEAD request to check if URL is accessible
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

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

func CollectMultipleFeeds(ctx context.Context, feedURLs []url.URL) ([]*domain.FeedItem, error) {
	fp := rssFeed.NewParser()
	var feeds []*rssFeed.Feed
	var errors []error

	for _, feedURL := range feedURLs {
		// First validate the URL
		if err := validateFeedURL(feedURL); err != nil {
			logger.Logger.Error("Feed URL validation failed", "url", feedURL.String(), "error", err)
			errors = append(errors, err)
			continue
		}

		feed, err := fp.ParseURL(feedURL.String())
		if err != nil {
			logger.Logger.Error("Error parsing feed", "url", feedURL.String(), "error", err)
			errors = append(errors, err)
			continue // Continue processing other feeds instead of failing entirely
		}

		feeds = append(feeds, feed)
		logger.Logger.Info("Successfully parsed feed", "url", feedURL.String(), "title", feed.Title)
	}

	// Log summary of collection results
	logger.Logger.Info("Feed collection summary", "successful", len(feeds), "failed", len(errors), "total", len(feedURLs))

	// Only return error if all feeds failed
	if len(feeds) == 0 && len(errors) > 0 {
		logger.Logger.Error("All feeds failed to parse", "total_errors", len(errors))
		return nil, errors[0] // Return the first error
	}

	feedItems := ConvertFeedToFeedItem(feeds)
	logger.Logger.Info("Feed items", "feedItems", feedItems)
	return feedItems, nil
}

func ConvertFeedToFeedItem(feeds []*rssFeed.Feed) []*domain.FeedItem {
	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		for _, item := range feed.Items {
			var author domain.Author
			var authors []domain.Author
			if item.Author != nil {
				author = domain.Author{Name: item.Author.Name}
				authors = append(authors, author)
			}
			feedItems = append(feedItems, &domain.FeedItem{
				Title:           item.Title,
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
