package reading_theme_gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/shared/driver/sovereign_client"
)

type stubThemeDriver struct {
	rows      []sovereign_client.ReadingThemeRow
	err       error
	seenUser  uuid.UUID
	seenLimit int32
}

func (s *stubThemeDriver) ListReadingThemes(_ context.Context, userID uuid.UUID, limit int32) ([]sovereign_client.ReadingThemeRow, error) {
	s.seenUser = userID
	s.seenLimit = limit
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func TestGateway_ListReadingThemes(t *testing.T) {
	userID := uuid.New()
	themeID := uuid.New()
	createdAt := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	openedAt := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	drv := &stubThemeDriver{
		rows: []sovereign_client.ReadingThemeRow{
			{
				ThemeID:           themeID,
				Title:             "Distributed Systems",
				LatestVersion:     3,
				ArticleCount:      7,
				CreatedAt:         createdAt,
				UpdatedAt:         updatedAt,
				LastOpenedAt:      &openedAt,
				LastOpenedVersion: 2,
			},
		},
	}

	gw := NewGateway(drv)
	got, err := gw.ListReadingThemes(context.Background(), userID, 0)
	require.NoError(t, err)
	assert.Equal(t, userID, drv.seenUser)
	assert.Equal(t, int32(0), drv.seenLimit)
	require.Len(t, got, 1)
	assert.Equal(t, themeID, got[0].ThemeID)
	assert.Equal(t, "Distributed Systems", got[0].Title)
	assert.Equal(t, int32(3), got[0].LatestVersion)
	assert.Equal(t, int32(7), got[0].ArticleCount)
	assert.Equal(t, createdAt, got[0].CreatedAt)
	assert.Equal(t, updatedAt, got[0].UpdatedAt)
	require.NotNil(t, got[0].LastOpenedAt)
	assert.Equal(t, openedAt, *got[0].LastOpenedAt)
	assert.Equal(t, int32(2), got[0].LastOpenedVersion)

	t.Run("propagates driver error", func(t *testing.T) {
		errDrv := &stubThemeDriver{err: errors.New("sovereign unavailable")}
		errGw := NewGateway(errDrv)
		_, err := errGw.ListReadingThemes(context.Background(), userID, 0)
		require.Error(t, err)
	})
}
