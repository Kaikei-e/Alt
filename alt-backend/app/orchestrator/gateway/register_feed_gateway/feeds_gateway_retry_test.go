package register_feed_gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/mmcdole/gofeed"
)

// Test retry behavior of DefaultRSSFeedFetcher (still defined in this package)
func TestDefaultRSSFeedFetcher_RetryableErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"timeout is retryable", errors.New("context deadline exceeded: timeout"), true},
		{"connection refused is retryable", errors.New("dial tcp: connection refused"), true},
		{"no such host is retryable", errors.New("dial tcp: lookup foo: no such host"), true},
		{"502 is retryable", errors.New("http error: 502 Bad Gateway"), true},
		{"503 is retryable", errors.New("http error: 503 Service Unavailable"), true},
		{"504 is retryable", errors.New("http error: 504 Gateway Timeout"), true},
		{"404 is not retryable", errors.New("http error: 404 Not Found"), false},
		{"parse error is not retryable", errors.New("failed to detect feed type"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryableError(%q) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestMockRSSFeedFetcher_Basic(t *testing.T) {
	fetcher := NewMockRSSFeedFetcher()

	// Default: returns a valid feed
	feed, err := fetcher.FetchRSSFeed(context.Background(), "https://example.com/rss")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if feed.Title != "Test Feed" {
		t.Errorf("expected 'Test Feed', got %q", feed.Title)
	}

	// SetError: returns error
	fetcher.SetError("https://fail.com/rss", errors.New("network error"))
	_, err = fetcher.FetchRSSFeed(context.Background(), "https://fail.com/rss")
	if err == nil {
		t.Fatal("expected error for fail URL")
	}

	// SetFeed: returns custom feed
	fetcher.SetFeed("https://custom.com/rss", &gofeed.Feed{Title: "Custom Feed", FeedLink: "https://custom.com/rss"})
	feed, err = fetcher.FetchRSSFeed(context.Background(), "https://custom.com/rss")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if feed.Title != "Custom Feed" {
		t.Errorf("expected 'Custom Feed', got %q", feed.Title)
	}
}
