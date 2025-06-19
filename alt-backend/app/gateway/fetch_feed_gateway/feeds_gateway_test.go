package fetch_feed_gateway

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
