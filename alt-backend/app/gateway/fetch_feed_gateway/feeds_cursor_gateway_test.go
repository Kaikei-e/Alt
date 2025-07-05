package fetch_feed_gateway

import (
	"alt/utils/logger"
	"context"
	"testing"
	"time"
)

func TestFetchFeedsGateway_FetchFeedsListCursor_NilCheck(t *testing.T) {
	logger.InitLogger()

	gateway := &FetchFeedsGateway{
		alt_db: nil,
	}

	ctx := context.Background()
	cursor := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name    string
		cursor  *time.Time
		limit   int
		wantErr bool
	}{
		{
			name:    "nil database connection - no cursor",
			cursor:  nil,
			limit:   10,
			wantErr: true,
		},
		{
			name:    "nil database connection - with cursor",
			cursor:  &cursor,
			limit:   5,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gateway.FetchFeedsListCursor(ctx, tt.cursor, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchFeedsGateway.FetchFeedsListCursor() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				expectedError := "database connection not available"
				if err.Error() != expectedError {
					t.Errorf("FetchFeedsGateway.FetchFeedsListCursor() error = %v, want %v", err.Error(), expectedError)
				}
			}
		})
	}
}

func TestFetchFeedsGateway_FetchReadFeedsListCursor_NilCheck(t *testing.T) {
	logger.InitLogger()

	gateway := &FetchFeedsGateway{
		alt_db: nil,
	}

	ctx := context.Background()
	cursor := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name    string
		cursor  *time.Time
		limit   int
		wantErr bool
	}{
		{
			name:    "nil database connection - no cursor",
			cursor:  nil,
			limit:   10,
			wantErr: true,
		},
		{
			name:    "nil database connection - with cursor",
			cursor:  &cursor,
			limit:   5,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gateway.FetchReadFeedsListCursor(ctx, tt.cursor, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchReadFeedsListCursor error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				expectedError := "database connection not available"
				if err.Error() != expectedError {
					t.Errorf("FetchReadFeedsListCursor error = %v, want %v", err.Error(), expectedError)
				}
			}
		})
	}
}

func TestFetchFeedsGateway_FetchFavoriteFeedsListCursor_NilCheck(t *testing.T) {
	logger.InitLogger()

	gateway := &FetchFeedsGateway{
		alt_db: nil,
	}

	ctx := context.Background()
	cursor := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name    string
		cursor  *time.Time
		limit   int
		wantErr bool
	}{
		{
			name:    "nil database connection - no cursor",
			cursor:  nil,
			limit:   10,
			wantErr: true,
		},
		{
			name:    "nil database connection - with cursor",
			cursor:  &cursor,
			limit:   5,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gateway.FetchFavoriteFeedsListCursor(ctx, tt.cursor, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchFavoriteFeedsListCursor error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				expectedError := "database connection not available"
				if err.Error() != expectedError {
					t.Errorf("FetchFavoriteFeedsListCursor error = %v, want %v", err.Error(), expectedError)
				}
			}
		})
	}
}
