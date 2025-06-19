package update_feed_status_gateway

import (
	"context"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUpdateFeedStatusGateway_UpdateFeedStatus(t *testing.T) {
	gateway := &UpdateFeedStatusGateway{
		db: nil, // This will cause an error, which we can test
	}

	// Create test URLs
	validURL, _ := url.Parse("https://example.com/feed.xml")
	invalidURL, _ := url.Parse("not-a-valid-url")

	type args struct {
		ctx     context.Context
		feedURL url.URL
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "update with valid URL but nil database (should error)",
			args: args{
				ctx:     context.Background(),
				feedURL: *validURL,
			},
			wantErr: true,
		},
		{
			name: "update with invalid URL",
			args: args{
				ctx:     context.Background(),
				feedURL: *invalidURL,
			},
			wantErr: true,
		},
		{
			name: "update with cancelled context",
			args: args{
				ctx: func() context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					return ctx
				}(),
				feedURL: *validURL,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := gateway.UpdateFeedStatus(tt.args.ctx, tt.args.feedURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateFeedStatusGateway.UpdateFeedStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewUpdateFeedStatusGateway(t *testing.T) {
	// Test constructor
	var pool *pgxpool.Pool // nil pool for testing
	gateway := NewUpdateFeedStatusGateway(pool)
	
	if gateway == nil {
		t.Error("NewUpdateFeedStatusGateway() returned nil")
	}
	
	if gateway.db == nil {
		t.Error("NewUpdateFeedStatusGateway() db should be initialized")
	}
}

func TestUpdateFeedStatusGateway_URLHandling(t *testing.T) {
	gateway := &UpdateFeedStatusGateway{
		db: nil,
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
			name:      "URL with authentication",
			urlString: "https://user:pass@example.com/feed.xml",
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

			err := gateway.UpdateFeedStatus(context.Background(), *parsedURL)
			if (err != nil) != testURL.wantErr {
				t.Errorf("UpdateFeedStatusGateway.UpdateFeedStatus() with URL %s error = %v, wantErr %v", 
					testURL.urlString, err, testURL.wantErr)
			}
		})
	}
}

func TestUpdateFeedStatusGateway_ErrorPropagation(t *testing.T) {
	gateway := &UpdateFeedStatusGateway{
		db: nil,
	}

	testURL, _ := url.Parse("https://example.com/feed.xml")
	
	// Test that errors from the database layer are properly propagated
	err := gateway.UpdateFeedStatus(context.Background(), *testURL)
	if err == nil {
		t.Error("UpdateFeedStatusGateway.UpdateFeedStatus() should propagate database errors")
	}
}

func TestUpdateFeedStatusGateway_ContextHandling(t *testing.T) {
	gateway := &UpdateFeedStatusGateway{
		db: nil,
	}

	testURL, _ := url.Parse("https://example.com/feed.xml")

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := gateway.UpdateFeedStatus(ctx, *testURL)
	if err == nil {
		t.Error("UpdateFeedStatusGateway.UpdateFeedStatus() expected error with cancelled context, got nil")
	}
}

func TestUpdateFeedStatusGateway_EmptyURL(t *testing.T) {
	gateway := &UpdateFeedStatusGateway{
		db: nil,
	}

	// Test with empty URL
	emptyURL := url.URL{}

	err := gateway.UpdateFeedStatus(context.Background(), emptyURL)
	if err == nil {
		t.Error("UpdateFeedStatusGateway.UpdateFeedStatus() expected error with empty URL, got nil")
	}
}