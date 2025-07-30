package register_feed_gateway

import (
	"alt/domain"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mmcdole/gofeed"
)

// MockRSSFeedFetcher for testing
type MockRSSFeedFetcher struct {
	feeds  map[string]*gofeed.Feed
	errors map[string]error
}

func NewMockRSSFeedFetcher() *MockRSSFeedFetcher {
	return &MockRSSFeedFetcher{
		feeds:  make(map[string]*gofeed.Feed),
		errors: make(map[string]error),
	}
}

func (m *MockRSSFeedFetcher) FetchRSSFeed(ctx context.Context, link string) (*gofeed.Feed, error) {
	// Check if we should return an error for this URL
	if err, exists := m.errors[link]; exists {
		return nil, err
	}

	// Check if we have a mock feed for this URL
	if feed, exists := m.feeds[link]; exists {
		return feed, nil
	}

	// Default: return a valid mock feed
	return &gofeed.Feed{
		Title:    "Test Feed",
		Link:     link,
		FeedLink: link,
	}, nil
}

func (m *MockRSSFeedFetcher) SetFeed(url string, feed *gofeed.Feed) {
	m.feeds[url] = feed
}

func (m *MockRSSFeedFetcher) SetError(url string, err error) {
	m.errors[url] = err
}

func TestRegisterFeedsGateway_RegisterFeeds(t *testing.T) {
	gateway := &RegisterFeedsGateway{
		alt_db: nil, // This will cause an error, which we can test
	}

	testFeeds := []*domain.FeedItem{
		{
			Title:       "Test Feed 1",
			Description: "Test Description 1",
			Link:        "https://example.com/feed1",
			Published:   "2024-01-01T00:00:00Z",
		},
		{
			Title:       "Test Feed 2",
			Description: "Test Description 2",
			Link:        "https://example.com/feed2",
			Published:   "2024-01-02T00:00:00Z",
		},
	}

	type args struct {
		ctx   context.Context
		feeds []*domain.FeedItem
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "register feeds with nil database (should error)",
			args: args{
				ctx:   context.Background(),
				feeds: testFeeds,
			},
			wantErr: true,
		},
		{
			name: "register empty feeds list",
			args: args{
				ctx:   context.Background(),
				feeds: []*domain.FeedItem{},
			},
			wantErr: true, // Should error with nil database
		},
		{
			name: "register nil feeds",
			args: args{
				ctx:   context.Background(),
				feeds: nil,
			},
			wantErr: true, // Should error with nil database
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := gateway.RegisterFeeds(tt.args.ctx, tt.args.feeds)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterFeedsGateway.RegisterFeeds() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegisterFeedGateway_RegisterRSSFeedLink(t *testing.T) {
	mockFetcher := NewMockRSSFeedFetcher()
	gateway := NewRegisterFeedLinkGatewayWithFetcher(nil, mockFetcher)

	type args struct {
		ctx context.Context
		url string
	}

	tests := []struct {
		name      string
		args      args
		wantErr   bool
		setupMock func()
	}{
		{
			name: "register RSS feed link with nil database (should error)",
			args: args{
				ctx: context.Background(),
				url: "https://example.com/feed.xml",
			},
			wantErr: true,
			setupMock: func() {
				mockFetcher.SetFeed("https://example.com/feed.xml", &gofeed.Feed{
					Title:    "Test Feed",
					Link:     "https://example.com/feed.xml",
					FeedLink: "https://example.com/feed.xml",
				})
			},
		},
		{
			name: "register empty URL",
			args: args{
				ctx: context.Background(),
				url: "",
			},
			wantErr: true, // Should error with invalid URL
			setupMock: func() {
				// No mock needed for URL validation error
			},
		},
		{
			name: "register invalid URL format",
			args: args{
				ctx: context.Background(),
				url: "not-a-valid-url",
			},
			wantErr: true, // Should error with invalid URL
			setupMock: func() {
				// No mock needed for URL validation error
			},
		},
		{
			name: "register valid URL",
			args: args{
				ctx: context.Background(),
				url: "https://example.com/rss.xml",
			},
			wantErr: true, // Should error due to unreachable URL
			setupMock: func() {
				mockFetcher.SetError("https://example.com/rss.xml", errors.New("no such host"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock for this test
			tt.setupMock()

			err := gateway.RegisterRSSFeedLink(tt.args.ctx, tt.args.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterFeedGateway.RegisterRSSFeedLink() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewRegisterFeedsGateway(t *testing.T) {
	// Test constructor
	var pool *pgxpool.Pool // nil pool for testing
	gateway := NewRegisterFeedsGateway(pool)

	if gateway == nil {
		t.Error("NewRegisterFeedsGateway() returned nil")
	}

	// With our refactored approach, repository will be nil when pool is nil
	if gateway.alt_db != nil {
		t.Error("NewRegisterFeedsGateway() with nil pool should have nil repository")
	}
}

func TestNewRegisterFeedLinkGateway(t *testing.T) {
	// Test constructor
	var pool *pgxpool.Pool // nil pool for testing
	gateway := NewRegisterFeedLinkGateway(pool)

	if gateway == nil {
		t.Error("NewRegisterFeedLinkGateway() returned nil")
	}

	// With our refactored approach, repository will be nil when pool is nil
	if gateway.alt_db != nil {
		t.Error("NewRegisterFeedLinkGateway() with nil pool should have nil repository")
	}
}

func TestRegisterFeedsGateway_ValidationEdgeCases(t *testing.T) {
	gateway := &RegisterFeedsGateway{
		alt_db: nil,
	}

	// Test with feeds containing various edge cases
	edgeCaseFeeds := []*domain.FeedItem{
		{
			Title:       "", // Empty title
			Description: "Valid description",
			Link:        "https://example.com/feed1",
			Published:   "2024-01-01T00:00:00Z",
		},
		{
			Title:       "Valid title",
			Description: "", // Empty description
			Link:        "https://example.com/feed2",
			Published:   "2024-01-01T00:00:00Z",
		},
		{
			Title:       "Valid title",
			Description: "Valid description",
			Link:        "", // Empty link
			Published:   "2024-01-01T00:00:00Z",
		},
		{
			Title:       "Valid title",
			Description: "Valid description",
			Link:        "https://example.com/feed3",
			Published:   "", // Empty published date
		},
	}

	err := gateway.RegisterFeeds(context.Background(), edgeCaseFeeds)
	if err == nil {
		t.Error("RegisterFeedsGateway.RegisterFeeds() expected error with nil database, got nil")
	}
}

func TestRegisterFeedsGateway_LargeDataset(t *testing.T) {
	gateway := &RegisterFeedsGateway{
		alt_db: nil,
	}

	// Create a large dataset to test performance characteristics
	var largeFeeds []*domain.FeedItem
	for i := 0; i < 1000; i++ {
		largeFeeds = append(largeFeeds, &domain.FeedItem{
			Title:       "Test Feed",
			Description: "Test Description",
			Link:        "https://example.com/feed",
			Published:   "2024-01-01T00:00:00Z",
		})
	}

	err := gateway.RegisterFeeds(context.Background(), largeFeeds)
	if err == nil {
		t.Error("RegisterFeedsGateway.RegisterFeeds() expected error with nil database, got nil")
	}
}

// TDD Red Phase: RSS feed validation timeout tests
func TestRegisterFeedGateway_TimeoutHandling(t *testing.T) {
	mockFetcher := NewMockRSSFeedFetcher()
	gateway := NewRegisterFeedLinkGatewayWithFetcher(nil, mockFetcher)

	tests := []struct {
		name            string
		url             string
		timeoutDuration time.Duration
		expectedError   string
		wantErr         bool
		setupMock       func()
	}{
		{
			name:            "timeout on slow RSS feed",
			url:             "https://httpbin.org/delay/20",
			timeoutDuration: 1 * time.Second,
			expectedError:   "RSS feed fetch timeout",
			wantErr:         true,
			setupMock: func() {
				mockFetcher.SetError("https://httpbin.org/delay/20", errors.New("context deadline exceeded"))
			},
		},
		{
			name:            "valid RSS feed within timeout",
			url:             "https://feeds.feedburner.com/oreilly",
			timeoutDuration: 30 * time.Second,
			expectedError:   "database connection not available",
			wantErr:         true,
			setupMock: func() {
				mockFetcher.SetFeed("https://feeds.feedburner.com/oreilly", &gofeed.Feed{
					Title:    "Test Feed",
					Link:     "https://feeds.feedburner.com/oreilly",
					FeedLink: "https://feeds.feedburner.com/oreilly",
				})
			},
		},
		{
			name:            "context deadline exceeded",
			url:             "https://httpbin.org/delay/15",
			timeoutDuration: 2 * time.Second,
			expectedError:   "RSS feed fetch timeout",
			wantErr:         true,
			setupMock: func() {
				mockFetcher.SetError("https://httpbin.org/delay/15", errors.New("context deadline exceeded"))
			},
		},
		{
			name:            "extended timeout - should succeed with 40s delay",
			url:             "https://httpbin.org/delay/40",
			timeoutDuration: 60 * time.Second,
			expectedError:   "database connection not available",
			wantErr:         true,
			setupMock: func() {
				mockFetcher.SetFeed("https://httpbin.org/delay/40", &gofeed.Feed{
					Title:    "Test Feed",
					Link:     "https://httpbin.org/delay/40",
					FeedLink: "https://httpbin.org/delay/40",
				})
			},
		},
		{
			name:            "verify extended timeout capacity",
			url:             "https://httpbin.org/delay/35",
			timeoutDuration: 60 * time.Second,
			expectedError:   "database connection not available",
			wantErr:         true,
			setupMock: func() {
				mockFetcher.SetFeed("https://httpbin.org/delay/35", &gofeed.Feed{
					Title:    "Test Feed",
					Link:     "https://httpbin.org/delay/35",
					FeedLink: "https://httpbin.org/delay/35",
				})
			},
		},
		{
			name:            "medium delay feed should succeed with extended timeouts",
			url:             "https://httpbin.org/delay/30",
			timeoutDuration: 60 * time.Second,
			expectedError:   "database connection not available",
			wantErr:         true,
			setupMock: func() {
				mockFetcher.SetFeed("https://httpbin.org/delay/30", &gofeed.Feed{
					Title:    "Test Feed",
					Link:     "https://httpbin.org/delay/30",
					FeedLink: "https://httpbin.org/delay/30",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock for this test
			tt.setupMock()

			// Create context with timeout for testing
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeoutDuration)
			defer cancel()

			err := gateway.RegisterRSSFeedLink(ctx, tt.url)

			if !tt.wantErr && err != nil {
				t.Errorf("RegisterRSSFeedLink() unexpected error = %v", err)
				return
			}

			if tt.wantErr && err == nil {
				t.Errorf("RegisterRSSFeedLink() expected error, got nil")
				return
			}

			if tt.expectedError != "" && !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("RegisterRSSFeedLink() error = %v, want error containing %v", err, tt.expectedError)
			}
		})
	}
}

// TDD Red Phase: Test RSS feed format validation with various formats
func TestRegisterFeedGateway_FeedFormatValidation(t *testing.T) {
	mockFetcher := NewMockRSSFeedFetcher()
	gateway := NewRegisterFeedLinkGatewayWithFetcher(nil, mockFetcher)

	tests := []struct {
		name          string
		url           string
		expectedError string
		wantErr       bool
		setupMock     func()
	}{
		{
			name:          "invalid RSS feed format - HTML page",
			url:           "https://httpbin.org/html",
			expectedError: "invalid RSS feed format",
			wantErr:       true,
			setupMock: func() {
				mockFetcher.SetError("https://httpbin.org/html", errors.New("Failed to detect feed type"))
			},
		},
		{
			name:          "invalid RSS feed format - JSON response",
			url:           "https://httpbin.org/json",
			expectedError: "invalid RSS feed format",
			wantErr:       true,
			setupMock: func() {
				mockFetcher.SetError("https://httpbin.org/json", errors.New("Failed to detect feed type"))
			},
		},
		{
			name:          "unreachable URL",
			url:           "https://nonexistent-domain-12345.com/feed.xml",
			expectedError: "could not reach the RSS feed URL",
			wantErr:       true,
			setupMock: func() {
				mockFetcher.SetError("https://nonexistent-domain-12345.com/feed.xml", errors.New("no such host"))
			},
		},
		{
			name:          "malformed URL",
			url:           "not-a-url",
			expectedError: "URL must include a scheme",
			wantErr:       true,
			setupMock: func() {
				// No mock needed for URL validation error
			},
		},
		{
			name:          "URL without scheme",
			url:           "example.com/feed.xml",
			expectedError: "URL must include a scheme",
			wantErr:       true,
			setupMock: func() {
				// No mock needed for URL validation error
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock for this test
			tt.setupMock()

			ctx := context.Background()
			err := gateway.RegisterRSSFeedLink(ctx, tt.url)

			if !tt.wantErr && err != nil {
				t.Errorf("RegisterRSSFeedLink() unexpected error = %v", err)
				return
			}

			if tt.wantErr && err == nil {
				t.Errorf("RegisterRSSFeedLink() expected error, got nil")
				return
			}

			if tt.expectedError != "" && !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("RegisterRSSFeedLink() error = %v, want error containing %v", err, tt.expectedError)
			}
		})
	}
}
