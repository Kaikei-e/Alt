package fetch_feed_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

func TestFetchFeedsGateway_FetchReadFeedsListCursor_MapsOgImage(t *testing.T) {
	logger.InitLogger()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	gateway := &FetchFeedsGateway{alt_db: alt_db.NewAltDBRepository(mock)}

	userID := uuid.New()
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	feedID := uuid.New().String()
	articleID := uuid.New().String()
	now := time.Now()
	ogURL := "https://img.example.com/read-og.jpg"

	mock.ExpectQuery(`(?s)article_heads.*INNER JOIN read_status`).
		WithArgs(20, userID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "title", "description", "website_url", "pub_date", "created_at", "updated_at", "article_id", "og_image_url",
		}).AddRow(feedID, "Read Title", "Desc", "https://example.com/feed", now, now, now, &articleID, &ogURL))

	items, err := gateway.FetchReadFeedsListCursor(ctx, nil, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, ogURL, items[0].OgImageURL)
	require.Equal(t, articleID, items[0].ArticleID)
	require.NoError(t, mock.ExpectationsWereMet())
}

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
			_, err := gateway.FetchFeedsListCursor(ctx, tt.cursor, tt.limit, nil)
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

func TestFetchFeedsGateway_FetchUnreadFeedsListCursor_NilCheck(t *testing.T) {
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
			_, err := gateway.FetchUnreadFeedsListCursor(ctx, tt.cursor, tt.limit, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchUnreadFeedsListCursor error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				expectedError := "database connection not available"
				if err.Error() != expectedError {
					t.Errorf("FetchUnreadFeedsListCursor error = %v, want %v", err.Error(), expectedError)
				}
			}
		})
	}
}
