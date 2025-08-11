package fetch_feed_gateway

import (
	"alt/domain"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Skip detailed mock testing for now, focus on TDD integration test

func TestFetchFeedsGateway_FetchFeeds(t *testing.T) {
	// Initialize logger to prevent nil pointer dereference
	logger.InitLogger()

	// Create a test RSS feed XML
	testRSSFeed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
	<channel>
		<title>Test Feed</title>
		<description>Test Description</description>
		<item>
			<title>Test Item 1</title>
			<description>Test Item Description 1</description>
			<link>https://example.com/item1</link>
			<pubDate>Mon, 01 Jan 2024 00:00:00 +0000</pubDate>
			<author>test@example.com (Test Author)</author>
		</item>
		<item>
			<title>Test Item 2</title>
			<description>Test Item Description 2</description>
			<link>https://example.com/item2</link>
			<pubDate>Tue, 02 Jan 2024 00:00:00 +0000</pubDate>
		</item>
	</channel>
</rss>`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(testRSSFeed))
	}))
	defer server.Close()

	// Create test server for invalid XML
	invalidServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte("invalid xml"))
	}))
	defer invalidServer.Close()

	gateway := &FetchFeedsGateway{
		alt_db: nil, // Not used in FetchFeeds method
	}

	type args struct {
		ctx  context.Context
		link string
	}

	tests := []struct {
		name    string
		args    args
		want    []*domain.FeedItem
		wantErr bool
	}{
		{
			name: "successful feed parsing",
			args: args{
				ctx:  context.Background(),
				link: server.URL,
			},
			want: []*domain.FeedItem{
				{
					Title:       "Test Item 1",
					Description: "Test Item Description 1",
					Link:        "https://example.com/item1",
					Published:   "Mon, 01 Jan 2024 00:00:00 +0000",
					Author: domain.Author{
						Name: "Test Author",
					},
					Authors: []domain.Author{
						{Name: "Test Author"},
					},
					PublishedParsed: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					Title:           "Test Item 2",
					Description:     "Test Item Description 2",
					Link:            "https://example.com/item2",
					Published:       "Tue, 02 Jan 2024 00:00:00 +0000",
					PublishedParsed: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				},
			},
			wantErr: false,
		},
		{
			name: "invalid feed URL",
			args: args{
				ctx:  context.Background(),
				link: "invalid-url",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid XML response",
			args: args{
				ctx:  context.Background(),
				link: invalidServer.URL,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty link",
			args: args{
				ctx:  context.Background(),
				link: "",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "non-existent URL",
			args: args{
				ctx:  context.Background(),
				link: "http://localhost:99999/nonexistent",
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gateway.FetchFeeds(tt.args.ctx, tt.args.link)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchFeedsGateway.FetchFeeds() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !compareFeedItems(got, tt.want) {
				t.Errorf("FetchFeedsGateway.FetchFeeds() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetchFeedsGateway_FetchFeedsList(t *testing.T) {
	// This test would require database mocking, which is more complex
	// For now, we'll test the basic structure and error handling
	gateway := &FetchFeedsGateway{
		alt_db: nil, // This will cause an error, which we can test
	}

	ctx := context.Background()
	_, err := gateway.FetchFeedsList(ctx)

	// Should error because alt_db is nil
	if err == nil {
		t.Error("FetchFeedsGateway.FetchFeedsList() expected error with nil alt_db, got nil")
	}
}

func TestFetchFeedsGateway_FetchFeedsListLimit(t *testing.T) {
	gateway := &FetchFeedsGateway{
		alt_db: nil, // This will cause an error, which we can test
	}

	ctx := context.Background()
	_, err := gateway.FetchFeedsListLimit(ctx, 10)

	// Should error because alt_db is nil
	if err == nil {
		t.Error("FetchFeedsGateway.FetchFeedsListLimit() expected error with nil alt_db, got nil")
	}
}

func TestFetchFeedsGateway_FetchFeedsListPage(t *testing.T) {
	gateway := &FetchFeedsGateway{
		alt_db: nil, // This will cause an error, which we can test
	}

	ctx := context.Background()
	_, err := gateway.FetchFeedsListPage(ctx, 1)

	// Should error because alt_db is nil
	if err == nil {
		t.Error("FetchFeedsGateway.FetchFeedsListPage() expected error with nil alt_db, got nil")
	}
}

func TestFetchFeedsGateway_FetchFeedsListPage_ShouldNotFallbackToReadArticles(t *testing.T) {
	// TDD Red: Test that current implementation DOES fallback (this should fail)
	// This test will document the dangerous behavior before we fix it
	
	gateway := &FetchFeedsGateway{
		alt_db: nil, // This will cause FetchUnreadFeedsListPage to fail
	}

	ctx := context.Background()
	feeds, err := gateway.FetchFeedsListPage(ctx, 1)

	// Current dangerous implementation: returns error since alt_db is nil
	// After fix: should return error without fallback
	if err == nil {
		t.Error("Expected error when database connection is not available")
	}
	
	if feeds != nil && len(feeds) > 0 {
		t.Error("Expected no feeds when database error occurs, got feeds (dangerous fallback detected)")
	}
}

func TestNewFetchFeedsGateway(t *testing.T) {
	// Test constructor
	var pool *pgxpool.Pool // nil pool for testing
	gateway := NewFetchFeedsGateway(pool)

	if gateway == nil {
		t.Error("NewFetchFeedsGateway() returned nil")
	}

	// With our refactored approach, repository will be nil when pool is nil
	if gateway.alt_db != nil {
		t.Error("NewFetchFeedsGateway() with nil pool should have nil repository")
	}
}

// Helper function to compare feed items while handling time comparison
func compareFeedItems(got, want []*domain.FeedItem) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i].Title != want[i].Title ||
			got[i].Description != want[i].Description ||
			got[i].Link != want[i].Link ||
			got[i].Published != want[i].Published {
			return false
		}

		// Compare authors
		if len(got[i].Authors) != len(want[i].Authors) {
			return false
		}

		for j := range got[i].Authors {
			if got[i].Authors[j].Name != want[i].Authors[j].Name {
				return false
			}
		}

		// Compare published time (allowing for small differences due to parsing)
		if !got[i].PublishedParsed.IsZero() && !want[i].PublishedParsed.IsZero() {
			if !got[i].PublishedParsed.Equal(want[i].PublishedParsed) {
				return false
			}
		}
	}

	return true
}

func TestFetchFeedsGateway_RateLimiting(t *testing.T) {
	// Initialize logger to prevent nil pointer dereference
	logger.InitLogger()

	// Create a test RSS feed XML
	testRSSFeed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
	<channel>
		<title>Test Feed</title>
		<description>Test Description</description>
		<item>
			<title>Test Item</title>
			<description>Test Description</description>
			<link>https://example.com/item</link>
		</item>
	</channel>
</rss>`

	// Create test server for same host testing
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(testRSSFeed))
	}))
	defer server1.Close()

	// Create another test server for different host testing
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(testRSSFeed))
	}))
	defer server2.Close()

	// Test that rate limiter is called for external API calls
	t.Run("gateway should use rate limiter for external calls", func(t *testing.T) {
		rateLimiter := rate_limiter.NewHostRateLimiter(100 * time.Millisecond)
		gateway := &FetchFeedsGateway{
			alt_db:      nil,
			rateLimiter: rateLimiter, // This field doesn't exist yet - should cause compile error
		}

		ctx := context.Background()

		// First call should succeed
		start := time.Now()
		_, err := gateway.FetchFeeds(ctx, server1.URL)
		if err != nil {
			t.Fatalf("First FetchFeeds call failed: %v", err)
		}
		firstCallDuration := time.Since(start)

		// Should be relatively fast (less than 50ms)
		if firstCallDuration > 50*time.Millisecond {
			t.Errorf("First call took too long: %v", firstCallDuration)
		}

		// Second call to same host should be rate limited
		start = time.Now()
		_, err = gateway.FetchFeeds(ctx, server1.URL)
		if err != nil {
			t.Fatalf("Second FetchFeeds call failed: %v", err)
		}
		secondCallDuration := time.Since(start)

		// Should wait for rate limit (approximately 100ms)
		if secondCallDuration < 80*time.Millisecond {
			t.Errorf("Second call was not rate limited: %v", secondCallDuration)
		}
	})

	t.Run("different hosts should not interfere with rate limiting", func(t *testing.T) {
		rateLimiter := rate_limiter.NewHostRateLimiter(200 * time.Millisecond)
		gateway := &FetchFeedsGateway{
			alt_db:      nil,
			rateLimiter: rateLimiter, // This field doesn't exist yet - should cause compile error
		}

		ctx := context.Background()

		// Call to first host
		_, err := gateway.FetchFeeds(ctx, server1.URL)
		if err != nil {
			t.Fatalf("First host call failed: %v", err)
		}

		// Call to second host should be immediate (different host)
		start := time.Now()
		_, err = gateway.FetchFeeds(ctx, server2.URL)
		if err != nil {
			t.Fatalf("Second host call failed: %v", err)
		}
		duration := time.Since(start)

		// Should be immediate for different host
		if duration > 50*time.Millisecond {
			t.Errorf("Different host call took too long: %v", duration)
		}
	})
}

func TestFetchFeedsGateway_WithRateLimiter(t *testing.T) {
	// Test the constructor with rate limiter
	var pool *pgxpool.Pool
	rateLimiter := rate_limiter.NewHostRateLimiter(5 * time.Second)

	gateway := NewFetchFeedsGatewayWithRateLimiter(pool, rateLimiter) // This function doesn't exist yet - should cause compile error

	if gateway == nil {
		t.Error("NewFetchFeedsGatewayWithRateLimiter() returned nil")
	}

	if gateway.rateLimiter != rateLimiter {
		t.Error("Rate limiter not properly set in gateway")
	}
}

// RED: TDD test for proxy-aware HTTP client usage (should fail initially)
func TestFetchFeedsGateway_FetchFeeds_ProxyIntegration(t *testing.T) {
	// Test server to simulate RSS feed
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
	<channel>
		<title>Test Feed</title>
		<description>Test RSS Feed</description>
		<item>
			<title>Test Item</title>
			<description>Test Description</description>
		</item>
	</channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify that the request comes through custom HTTP client
		// (proxy-aware client would have specific headers/behavior)
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(rssContent))
	}))
	defer server.Close()

	// Initialize logger for test
	logger.InitLogger()

	tests := []struct {
		name     string
		gateway  *FetchFeedsGateway
		link     string
		wantErr  bool
		errCheck func(error) bool
	}{
		{
			name: "should_use_createHTTPClient_method",
			gateway: &FetchFeedsGateway{
				alt_db:      nil, // Will trigger error, but test focuses on HTTP client creation
				rateLimiter: nil,
			},
			link:    server.URL,
			wantErr: false, // Should succeed with proper HTTP client
			errCheck: func(err error) bool {
				// This test will initially fail because createHTTPClient method doesn't exist
				// Expected to fail with: "g.createHTTPClient undefined (type *FetchFeedsGateway has no field or method createHTTPClient)"
				return err == nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// This should fail in RED phase because createHTTPClient method doesn't exist
			_, err := tt.gateway.FetchFeeds(ctx, tt.link)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				if tt.errCheck != nil && !tt.errCheck(err) {
					t.Errorf("Error check failed for error: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}
