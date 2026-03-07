package validate_fetch_rss_gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFetcher for testing
type mockFetcher struct {
	feeds  map[string]*gofeed.Feed
	errors map[string]error
}

func newMockFetcher() *mockFetcher {
	return &mockFetcher{
		feeds:  make(map[string]*gofeed.Feed),
		errors: make(map[string]error),
	}
}

func (m *mockFetcher) FetchRSSFeed(ctx context.Context, link string) (*gofeed.Feed, error) {
	if err, exists := m.errors[link]; exists {
		return nil, err
	}
	if feed, exists := m.feeds[link]; exists {
		return feed, nil
	}
	return &gofeed.Feed{
		Title:    "Test Feed",
		Link:     link,
		FeedLink: link,
	}, nil
}

func (m *mockFetcher) setFeed(url string, feed *gofeed.Feed) {
	m.feeds[url] = feed
}

func (m *mockFetcher) setError(url string, err error) {
	m.errors[url] = err
}

func TestValidateAndFetchRSSGateway_ValidateAndFetch_Success(t *testing.T) {
	fetcher := newMockFetcher()
	now := time.Now()

	fetcher.setFeed("https://example.com/feed.xml", &gofeed.Feed{
		Title:    "Example Feed",
		Link:     "https://example.com",
		FeedLink: "https://example.com/feed.xml",
		Items: []*gofeed.Item{
			{
				Title:           "Article 1",
				Description:     "Description 1",
				Link:            "https://example.com/article1",
				Published:       "2025-01-01T00:00:00Z",
				PublishedParsed: &now,
				Author:          &gofeed.Person{Name: "Author 1"},
				Links:           []string{"https://example.com/article1"},
			},
			{
				Title:       "Article 2",
				Description: "Description 2",
				Link:        "https://example.com/article2",
			},
		},
	})

	gw := NewValidateAndFetchRSSGatewayWithFetcher(fetcher)

	result, err := gw.ValidateAndFetch(context.Background(), "https://example.com/feed.xml")

	require.NoError(t, err)
	assert.Equal(t, "https://example.com/feed.xml", result.FeedLink)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "Article 1", result.Items[0].Title)
	assert.Equal(t, "Author 1", result.Items[0].Author.Name)
	assert.Equal(t, "Article 2", result.Items[1].Title)
}

func TestValidateAndFetchRSSGateway_EmptyFeedLink_UsesInputURL(t *testing.T) {
	fetcher := newMockFetcher()
	fetcher.setFeed("https://example.com/rss", &gofeed.Feed{
		Title:    "Test",
		Link:     "https://example.com",
		FeedLink: "", // Empty feed link
		Items:    []*gofeed.Item{},
	})

	gw := NewValidateAndFetchRSSGatewayWithFetcher(fetcher)

	result, err := gw.ValidateAndFetch(context.Background(), "https://example.com/rss")

	require.NoError(t, err)
	assert.Equal(t, "https://example.com/rss", result.FeedLink)
}

func TestValidateAndFetchRSSGateway_SecurityValidation(t *testing.T) {
	fetcher := newMockFetcher()
	gw := NewValidateAndFetchRSSGatewayWithFetcher(fetcher)

	maliciousURLs := []struct {
		url           string
		expectedError string
	}{
		{"http://192.168.1.1/feed.xml", "private network access denied"},
		{"http://10.0.0.1/feed.xml", "private network access denied"},
		{"http://localhost/feed.xml", "private network access denied"},
		{"http://127.0.0.1/feed.xml", "private network access denied"},
		{"ftp://example.com/feed.xml", "only HTTP and HTTPS schemes allowed"},
		{"javascript:alert('xss')", "only HTTP and HTTPS schemes allowed"},
		{"file:///etc/passwd", "only HTTP and HTTPS schemes allowed"},
	}

	for _, tt := range maliciousURLs {
		t.Run("should block "+tt.url, func(t *testing.T) {
			_, err := gw.ValidateAndFetch(context.Background(), tt.url)

			assert.Error(t, err)
			assert.True(t,
				strings.Contains(err.Error(), tt.expectedError),
				"Error for %s should contain %q, got: %s", tt.url, tt.expectedError, err.Error())
		})
	}
}

func TestValidateAndFetchRSSGateway_FetchErrors(t *testing.T) {
	tests := []struct {
		name          string
		fetchErr      error
		expectedError string
	}{
		{"connection error", errors.New("no such host"), "could not reach the RSS feed URL"},
		{"timeout error", errors.New("context deadline exceeded"), "RSS feed fetch timeout"},
		{"404 error", errors.New("http error: 404 Not Found"), "RSS feed not found (404)"},
		{"403 error", errors.New("http error: 403 Forbidden"), "RSS feed access forbidden (403)"},
		{"format error", errors.New("failed to detect feed type"), "invalid RSS feed format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := newMockFetcher()
			fetcher.setError("https://example.com/feed.xml", tt.fetchErr)

			gw := NewValidateAndFetchRSSGatewayWithFetcher(fetcher)

			_, err := gw.ValidateAndFetch(context.Background(), "https://example.com/feed.xml")

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestValidateAndFetchRSSGateway_Semaphore_ContextCancellation(t *testing.T) {
	fetcher := newMockFetcher()
	// Create gateway with semaphore of 1
	gw := &ValidateAndFetchRSSGateway{
		feedFetcher:      fetcher,
		urlValidator:     nil, // Skip validation for this test
		circuitBreaker:   nil,
		metricsCollector: nil,
		fetchSem:         make(chan struct{}, 1),
	}

	// Fill the semaphore
	gw.fetchSem <- struct{}{}

	// Try with already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := gw.ValidateAndFetch(ctx, "https://example.com/feed.xml")
	assert.Error(t, err)

	// Release semaphore
	<-gw.fetchSem
}

func TestValidateAndFetchRSSGateway_Metrics(t *testing.T) {
	fetcher := newMockFetcher()
	gw := NewValidateAndFetchRSSGatewayWithFetcher(fetcher)

	// Successful fetch
	fetcher.setFeed("https://example.com/feed.xml", &gofeed.Feed{
		Title:    "Test",
		FeedLink: "https://example.com/feed.xml",
		Items:    []*gofeed.Item{},
	})

	_, err := gw.ValidateAndFetch(context.Background(), "https://example.com/feed.xml")
	require.NoError(t, err)

	metrics := gw.GetMetrics()
	assert.Equal(t, int64(1), metrics.GetSuccessfulRequests())
	assert.Equal(t, int64(0), metrics.GetFailedRequests())

	// Failed fetch
	fetcher.setError("https://fail.com/feed.xml", errors.New("no such host"))
	_, err = gw.ValidateAndFetch(context.Background(), "https://fail.com/feed.xml")
	assert.Error(t, err)

	assert.Equal(t, int64(1), metrics.GetSuccessfulRequests())
	assert.Equal(t, int64(1), metrics.GetFailedRequests())
}
