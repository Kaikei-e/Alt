package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestListReadingThemes_ValidAndInvalidRequests(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	themeOpened := uuid.New()
	themeUnopened := uuid.New()
	t1 := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	openedAt := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		readingThemes: []sovereign_db.ReadingTheme{
			{
				UserID:            userID,
				TenantID:          tenantID,
				ThemeID:           themeOpened,
				Title:             "Distributed Storage",
				LatestVersion:     3,
				ArticleCount:      5,
				CreatedAt:         t1,
				UpdatedAt:         t2,
				LastOpenedAt:      &openedAt,
				LastOpenedVersion: 2,
			},
			{
				UserID:            userID,
				TenantID:          tenantID,
				ThemeID:           themeUnopened,
				Title:             "Go Compilers",
				LatestVersion:     1,
				ArticleCount:      2,
				CreatedAt:         t1,
				UpdatedAt:         t1,
				LastOpenedAt:      nil,
				LastOpenedVersion: 0,
			},
		},
	}
	h := NewSovereignHandler(repo)

	t.Run("rejects invalid user_id", func(t *testing.T) {
		_, err := h.ListReadingThemes(context.Background(), connect.NewRequest(&sovereignv1.ListReadingThemesRequest{
			UserId: "bad-uuid",
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("defaults limit 0 to 100 and caps above 500", func(t *testing.T) {
		_, err := h.ListReadingThemes(context.Background(), connect.NewRequest(&sovereignv1.ListReadingThemesRequest{
			UserId: userID.String(),
			Limit:  0,
		}))
		require.NoError(t, err)
		assert.Equal(t, 100, repo.gotThemesLimit)

		_, err = h.ListReadingThemes(context.Background(), connect.NewRequest(&sovereignv1.ListReadingThemesRequest{
			UserId: userID.String(),
			Limit:  999,
		}))
		require.NoError(t, err)
		assert.Equal(t, 500, repo.gotThemesLimit)
	})

	t.Run("maps theme fields and nullable last_opened_at", func(t *testing.T) {
		resp, err := h.ListReadingThemes(context.Background(), connect.NewRequest(&sovereignv1.ListReadingThemesRequest{
			UserId: userID.String(),
			Limit:  20,
		}))
		require.NoError(t, err)
		assert.Equal(t, 20, repo.gotThemesLimit)
		require.Len(t, resp.Msg.Themes, 2)

		first := resp.Msg.Themes[0]
		assert.Equal(t, themeOpened.String(), first.ThemeId)
		assert.Equal(t, "Distributed Storage", first.Title)
		assert.Equal(t, int32(3), first.LatestVersion)
		assert.Equal(t, int32(5), first.ArticleCount)
		assert.Equal(t, t1, first.CreatedAt.AsTime())
		assert.Equal(t, t2, first.UpdatedAt.AsTime())
		require.NotNil(t, first.LastOpenedAt)
		assert.Equal(t, openedAt, first.LastOpenedAt.AsTime())
		assert.Equal(t, int32(2), first.LastOpenedVersion)

		second := resp.Msg.Themes[1]
		assert.Equal(t, themeUnopened.String(), second.ThemeId)
		assert.Nil(t, second.LastOpenedAt)
		assert.Equal(t, int32(0), second.LastOpenedVersion)
	})

	t.Run("propagates repository error as Internal", func(t *testing.T) {
		errRepo := &mockRepo{returnErr: errors.New("db unavailable")}
		errH := NewSovereignHandler(errRepo)

		_, err := errH.ListReadingThemes(context.Background(), connect.NewRequest(&sovereignv1.ListReadingThemesRequest{
			UserId: userID.String(),
		}))
		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})
}
