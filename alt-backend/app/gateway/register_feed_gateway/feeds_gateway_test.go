package register_feed_gateway

import (
	"alt/domain"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
	gateway := &RegisterFeedGateway{
		alt_db: nil, // This will cause an error, which we can test
	}

	type args struct {
		ctx context.Context
		url string
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "register RSS feed link with nil database (should error)",
			args: args{
				ctx: context.Background(),
				url: "https://example.com/feed.xml",
			},
			wantErr: true,
		},
		{
			name: "register empty URL",
			args: args{
				ctx: context.Background(),
				url: "",
			},
			wantErr: true, // Should error with invalid URL
		},
		{
			name: "register invalid URL format",
			args: args{
				ctx: context.Background(),
				url: "not-a-valid-url",
			},
			wantErr: true, // Should error with invalid URL
		},
		{
			name: "register valid URL",
			args: args{
				ctx: context.Background(),
				url: "https://example.com/rss.xml",
			},
			wantErr: true, // Should error due to unreachable URL
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	gateway := &RegisterFeedGateway{
		alt_db: nil, // Database will be mocked for timeout testing
	}

	tests := []struct {
		name            string
		url             string
		timeoutDuration time.Duration
		expectedError   string
		wantErr         bool
	}{
		{
			name:            "timeout on slow RSS feed",
			url:             "https://httpbin.org/delay/20", // Simulates 20 second delay
			timeoutDuration: 1 * time.Second,
			expectedError:   "timeout",
			wantErr:         true,
		},
		{
			name:            "valid RSS feed within timeout",
			url:             "https://feeds.feedburner.com/oreilly", // Real RSS feed
			timeoutDuration: 30 * time.Second,
			expectedError:   "",
			wantErr:         true, // Still expect error due to nil database
		},
		{
			name:            "context deadline exceeded",
			url:             "https://httpbin.org/delay/15",
			timeoutDuration: 2 * time.Second,
			expectedError:   "timeout",
			wantErr:         true,
		},
		{
			name:            "extended timeout - should succeed with 40s delay",
			url:             "https://httpbin.org/delay/40", // 40 second delay - should succeed
			timeoutDuration: 60 * time.Second,
			expectedError:   "database connection not available",
			wantErr:         true, // Should succeed RSS fetch but fail at database level
		},
		{
			name:            "verify extended timeout capacity",
			url:             "https://httpbin.org/delay/35", // 35 second delay - should succeed
			timeoutDuration: 60 * time.Second,
			expectedError:   "database connection not available",
			wantErr:         true, // Should succeed RSS fetch but fail at database level
		},
		{
			name:            "medium delay feed should succeed with extended timeouts",
			url:             "https://httpbin.org/delay/30", // 30 second delay
			timeoutDuration: 60 * time.Second,
			expectedError:   "database connection not available",
			wantErr:         true, // Should succeed RSS fetch but fail at database level
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	gateway := &RegisterFeedGateway{
		alt_db: nil,
	}

	tests := []struct {
		name          string
		url           string
		expectedError string
		wantErr       bool
	}{
		{
			name:          "invalid RSS feed format - HTML page",
			url:           "https://httpbin.org/html",
			expectedError: "invalid RSS feed format",
			wantErr:       true,
		},
		{
			name:          "invalid RSS feed format - JSON response",
			url:           "https://httpbin.org/json",
			expectedError: "database connection not available",
			wantErr:       true,
		},
		{
			name:          "unreachable URL",
			url:           "https://nonexistent-domain-12345.com/feed.xml",
			expectedError: "could not reach the RSS feed URL",
			wantErr:       true,
		},
		{
			name:          "malformed URL",
			url:           "not-a-url",
			expectedError: "URL must include a scheme",
			wantErr:       true,
		},
		{
			name:          "URL without scheme",
			url:           "example.com/feed.xml",
			expectedError: "URL must include a scheme",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
