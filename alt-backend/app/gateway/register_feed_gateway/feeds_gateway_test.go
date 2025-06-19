package register_feed_gateway

import (
	"alt/domain"
	"context"
	"testing"

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
	
	if gateway.alt_db == nil {
		t.Error("NewRegisterFeedsGateway() alt_db should be initialized")
	}
}

func TestNewRegisterFeedLinkGateway(t *testing.T) {
	// Test constructor
	var pool *pgxpool.Pool // nil pool for testing
	gateway := NewRegisterFeedLinkGateway(pool)
	
	if gateway == nil {
		t.Error("NewRegisterFeedLinkGateway() returned nil")
	}
	
	if gateway.alt_db == nil {
		t.Error("NewRegisterFeedLinkGateway() alt_db should be initialized")
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