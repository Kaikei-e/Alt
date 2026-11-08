package datahub_capability_gateway

import (
	"context"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTagTrailDriver struct {
	articles   []*domain.TagTrailArticle
	err        error
	gotTagID   string
	gotTagName string
	gotUserID  *uuid.UUID
	gotCursor  *time.Time
	gotLimit   int
}

func (m *mockTagTrailDriver) FetchArticlesByTagForUser(_ context.Context, tagID string, userID *uuid.UUID, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	m.gotTagID = tagID
	m.gotUserID = userID
	m.gotCursor = cursor
	m.gotLimit = limit
	return m.articles, m.err
}

func (m *mockTagTrailDriver) FetchArticlesByTagNameForUser(_ context.Context, tagName string, userID *uuid.UUID, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	m.gotTagName = tagName
	m.gotUserID = userID
	m.gotCursor = cursor
	m.gotLimit = limit
	return m.articles, m.err
}

func TestTagTrailGateway_ArticlesByTagID_ForwardsUserID(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	cursor := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	drv := &mockTagTrailDriver{
		articles: []*domain.TagTrailArticle{{ID: "a1", Title: "Tagged"}},
	}
	gw := &TagTrailGateway{db: drv}

	got, err := gw.ArticlesByTagID(context.Background(), "tag-1", &userID, &cursor, 20)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "tag-1", drv.gotTagID)
	require.NotNil(t, drv.gotUserID)
	assert.Equal(t, userID, *drv.gotUserID)
	require.NotNil(t, drv.gotCursor)
	assert.Equal(t, cursor, *drv.gotCursor)
	assert.Equal(t, 20, drv.gotLimit)
}

func TestTagTrailGateway_ArticlesByTagName_ForwardsUserID(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	cursor := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	drv := &mockTagTrailDriver{
		articles: []*domain.TagTrailArticle{{ID: "a1", Title: "Cross-feed"}},
	}
	gw := &TagTrailGateway{db: drv}

	got, err := gw.ArticlesByTagName(context.Background(), "golang", &userID, &cursor, 15)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "golang", drv.gotTagName)
	require.NotNil(t, drv.gotUserID)
	assert.Equal(t, userID, *drv.gotUserID)
	require.NotNil(t, drv.gotCursor)
	assert.Equal(t, cursor, *drv.gotCursor)
	assert.Equal(t, 15, drv.gotLimit)
}
