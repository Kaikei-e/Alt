package fetch_feed_gateway

import (
	"alt/orchestrator/driver/models"

	stderrors "errors"

	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestFetchFeedsGateway_FetchReadFeedsListCursor_MapsOgImage(t *testing.T) {
	logger.InitLogger()

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

	// The og:image and the article id are pointers on the row and plain
	// strings on the item. That flattening is this gateway's job and the
	// reason the test exists: a nil pointer has to become "" rather than
	// panic, and a set one has to survive.
	gateway := &FetchFeedsGateway{store: &feedListStoreStub{feeds: []*models.Feed{{
		ID:          feedID,
		Title:       "Read Title",
		Description: "Desc",
		WebsiteURL:  "https://example.com/feed",
		PubDate:     now,
		CreatedAt:   now,
		UpdatedAt:   now,
		ArticleID:   &articleID,
		OgImageURL:  &ogURL,
	}}}}

	items, err := gateway.FetchReadFeedsListCursor(ctx, nil, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, ogURL, items[0].OgImageURL)
	require.Equal(t, articleID, items[0].ArticleID)
}

func TestFetchFeedsGateway_FetchFeedsListCursor_PropagatesStoreFailure(t *testing.T) {
	logger.InitLogger()

	gateway := &FetchFeedsGateway{
		store: &feedListStoreStub{err: stderrors.New("data plane unavailable")},
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
			name:    "store failure - no cursor",
			cursor:  nil,
			limit:   10,
			wantErr: true,
		},
		{
			name:    "store failure - with cursor",
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

			// The message is the gateway's, not the store's: the old test
			// asserted "database connection not available", which only ever
			// held because the gateway short-circuited on a nil repository.
			// What matters now is that a data plane failure reaches the caller
			// as a failure rather than as an empty list.
			if err != nil && len(err.Error()) == 0 {
				t.Error("expected a non-empty error message")
			}
		})
	}
}

func TestFetchFeedsGateway_FetchReadFeedsListCursor_PropagatesStoreFailure(t *testing.T) {
	logger.InitLogger()

	gateway := &FetchFeedsGateway{
		store: &feedListStoreStub{err: stderrors.New("data plane unavailable")},
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
			name:    "store failure - no cursor",
			cursor:  nil,
			limit:   10,
			wantErr: true,
		},
		{
			name:    "store failure - with cursor",
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

			// The message is the gateway's, not the store's: the old test
			// asserted "database connection not available", which only ever
			// held because the gateway short-circuited on a nil repository.
			// What matters now is that a data plane failure reaches the caller
			// as a failure rather than as an empty list.
			if err != nil && len(err.Error()) == 0 {
				t.Error("expected a non-empty error message")
			}
		})
	}
}

func TestFetchFeedsGateway_FetchFavoriteFeedsListCursor_PropagatesStoreFailure(t *testing.T) {
	logger.InitLogger()

	gateway := &FetchFeedsGateway{
		store: &feedListStoreStub{err: stderrors.New("data plane unavailable")},
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
			name:    "store failure - no cursor",
			cursor:  nil,
			limit:   10,
			wantErr: true,
		},
		{
			name:    "store failure - with cursor",
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

			// The message is the gateway's, not the store's: the old test
			// asserted "database connection not available", which only ever
			// held because the gateway short-circuited on a nil repository.
			// What matters now is that a data plane failure reaches the caller
			// as a failure rather than as an empty list.
			if err != nil && len(err.Error()) == 0 {
				t.Error("expected a non-empty error message")
			}
		})
	}
}

func TestFetchFeedsGateway_FetchUnreadFeedsListCursor_PropagatesStoreFailure(t *testing.T) {
	logger.InitLogger()

	gateway := &FetchFeedsGateway{
		store: &feedListStoreStub{err: stderrors.New("data plane unavailable")},
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
			name:    "store failure - no cursor",
			cursor:  nil,
			limit:   10,
			wantErr: true,
		},
		{
			name:    "store failure - with cursor",
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

			// The message is the gateway's, not the store's: the old test
			// asserted "database connection not available", which only ever
			// held because the gateway short-circuited on a nil repository.
			// What matters now is that a data plane failure reaches the caller
			// as a failure rather than as an empty list.
			if err != nil && len(err.Error()) == 0 {
				t.Error("expected a non-empty error message")
			}
		})
	}
}
