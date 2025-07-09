package feed_url_to_id_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"testing"
)

func TestFeedURLToIDGateway_GetFeedIDByURL_NilCheck(t *testing.T) {
	logger.InitLogger()

	gateway := &FeedURLToIDGateway{
		alt_db: nil,
	}

	ctx := context.Background()
	feedURL := "https://example.com/rss.xml"

	tests := []struct {
		name    string
		feedURL string
		wantErr bool
	}{
		{
			name:    "nil database connection",
			feedURL: feedURL,
			wantErr: true,
		},
		{
			name:    "nil database connection - empty URL",
			feedURL: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gateway.GetFeedIDByURL(ctx, tt.feedURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("FeedURLToIDGateway.GetFeedIDByURL() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				expectedError := "database connection not available"
				if err.Error() != expectedError {
					t.Errorf("FeedURLToIDGateway.GetFeedIDByURL() error = %v, want %v", err.Error(), expectedError)
				}
			}
		})
	}
}

func TestFeedURLToIDGateway_GetFeedIDByURL_DatabaseError(t *testing.T) {
	logger.InitLogger()

	// Test with invalid repository (nil pool) to test error handling
	gateway := &FeedURLToIDGateway{
		alt_db: &alt_db.AltDBRepository{}, // This will cause a database error
	}

	ctx := context.Background()
	feedURL := "https://example.com/rss.xml"

	// This should error due to invalid database connection
	feedID, err := gateway.GetFeedIDByURL(ctx, feedURL)
	if err == nil {
		t.Errorf("FeedURLToIDGateway.GetFeedIDByURL() error = nil, want error")
	}

	// Should return empty string on error
	if feedID != "" {
		t.Errorf("FeedURLToIDGateway.GetFeedIDByURL() returned %v, want empty string", feedID)
	}
}
