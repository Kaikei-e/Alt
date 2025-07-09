package fetch_feed_tags_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFetchFeedTagsGateway_FetchFeedTags_NilCheck(t *testing.T) {
	logger.InitLogger()

	gateway := &FetchFeedTagsGateway{
		alt_db: nil,
	}

	ctx := context.Background()
	feedID := uuid.New().String()

	tests := []struct {
		name    string
		feedID  string
		cursor  *time.Time
		limit   int
		wantErr bool
	}{
		{
			name:    "nil database connection",
			feedID:  feedID,
			cursor:  nil,
			limit:   20,
			wantErr: true,
		},
		{
			name:    "nil database connection - edge case",
			feedID:  feedID,
			cursor:  nil,
			limit:   1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gateway.FetchFeedTags(ctx, tt.feedID, tt.cursor, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchFeedTagsGateway.FetchFeedTags() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				expectedError := "database connection not available"
				if err.Error() != expectedError {
					t.Errorf("FetchFeedTagsGateway.FetchFeedTags() error = %v, want %v", err.Error(), expectedError)
				}
			}
		})
	}
}

func TestFetchFeedTagsGateway_FetchFeedTags_DatabaseError(t *testing.T) {
	logger.InitLogger()

	// Test with invalid repository (nil pool) to test error handling
	gateway := &FetchFeedTagsGateway{
		alt_db: &alt_db.AltDBRepository{}, // This will cause a database error
	}

	ctx := context.Background()
	feedID := uuid.New().String()
	limit := 20

	// This should error due to invalid database connection
	tags, err := gateway.FetchFeedTags(ctx, feedID, nil, limit)
	if err == nil {
		t.Errorf("FetchFeedTagsGateway.FetchFeedTags() error = nil, want error")
	}

	// Should return nil on error
	if tags != nil {
		t.Errorf("FetchFeedTagsGateway.FetchFeedTags() returned %v, want nil", tags)
	}
}
