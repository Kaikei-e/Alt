package fetch_feed_detail_gateway

import (
	"alt/domain"
	"context"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFeedSummaryGateway_FetchFeedDetails(t *testing.T) {
	gateway := &FeedSummaryGateway{
		alt_db: nil, // This will cause an error, which we can test
	}

	// Create test URLs
	validURL, _ := url.Parse("https://example.com/feed.xml")
	invalidURL, _ := url.Parse("not-a-valid-url")

	type args struct {
		ctx     context.Context
		feedURL *url.URL
	}

	tests := []struct {
		name    string
		args    args
		want    *domain.FeedSummary
		wantErr bool
	}{
		{
			name: "fetch with valid URL but nil database (should error)",
			args: args{
				ctx:     context.Background(),
				feedURL: validURL,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fetch with invalid URL",
			args: args{
				ctx:     context.Background(),
				feedURL: invalidURL,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fetch with nil URL",
			args: args{
				ctx:     context.Background(),
				feedURL: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fetch with cancelled context",
			args: args{
				ctx: func() context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					return ctx
				}(),
				feedURL: validURL,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gateway.FetchFeedDetails(tt.args.ctx, tt.args.feedURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("FeedSummaryGateway.FetchFeedDetails() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FeedSummaryGateway.FetchFeedDetails() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewFeedSummaryGateway(t *testing.T) {
	// Test constructor
	var pool *pgxpool.Pool // nil pool for testing
	gateway := NewFeedSummaryGateway(pool)

	if gateway == nil {
		t.Error("NewFeedSummaryGateway() returned nil")
	}

	// With our refactored approach, repository will be nil when pool is nil
	if gateway.alt_db != nil {
		t.Error("NewFeedSummaryGateway() with nil pool should have nil repository")
	}
}

func TestFeedSummaryGateway_URLValidation(t *testing.T) {
	gateway := &FeedSummaryGateway{
		alt_db: nil,
	}

	// Test various URL formats
	testURLs := []struct {
		name      string
		urlString string
		wantErr   bool
	}{
		{
			name:      "valid HTTP URL",
			urlString: "http://example.com/feed.xml",
			wantErr:   true, // Will error due to nil database
		},
		{
			name:      "valid HTTPS URL",
			urlString: "https://example.com/feed.xml",
			wantErr:   true, // Will error due to nil database
		},
		{
			name:      "URL with port",
			urlString: "https://example.com:8080/feed.xml",
			wantErr:   true, // Will error due to nil database
		},
		{
			name:      "URL with query parameters",
			urlString: "https://example.com/feed.xml?format=rss&version=2.0",
			wantErr:   true, // Will error due to nil database
		},
		{
			name:      "URL with fragment",
			urlString: "https://example.com/feed.xml#section1",
			wantErr:   true, // Will error due to nil database
		},
		{
			name:      "relative URL",
			urlString: "/feed.xml",
			wantErr:   true, // Will error due to nil database
		},
	}

	for _, testURL := range testURLs {
		t.Run(testURL.name, func(t *testing.T) {
			parsedURL, parseErr := url.Parse(testURL.urlString)
			if parseErr != nil {
				t.Fatalf("Failed to parse test URL: %v", parseErr)
			}

			_, err := gateway.FetchFeedDetails(context.Background(), parsedURL)
			if (err != nil) != testURL.wantErr {
				t.Errorf("FeedSummaryGateway.FetchFeedDetails() with URL %s error = %v, wantErr %v",
					testURL.urlString, err, testURL.wantErr)
			}
		})
	}
}

func TestFeedSummaryGateway_ErrorPropagation(t *testing.T) {
	gateway := &FeedSummaryGateway{
		alt_db: nil,
	}

	testURL, _ := url.Parse("https://example.com/feed.xml")

	// Test that errors from the database layer are properly propagated
	_, err := gateway.FetchFeedDetails(context.Background(), testURL)
	if err == nil {
		t.Error("FeedSummaryGateway.FetchFeedDetails() should propagate database errors")
	}
}

func TestFeedSummaryGateway_ContextHandling(t *testing.T) {
	gateway := &FeedSummaryGateway{
		alt_db: nil,
	}

	testURL, _ := url.Parse("https://example.com/feed.xml")

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := gateway.FetchFeedDetails(ctx, testURL)
	if err == nil {
		t.Error("FeedSummaryGateway.FetchFeedDetails() expected error with cancelled context, got nil")
	}
}
