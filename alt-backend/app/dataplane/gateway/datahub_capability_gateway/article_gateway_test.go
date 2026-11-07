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

type mockArticleReadDriver struct {
	gotArticleID string
	gotUserID    *uuid.UUID
	content      *domain.ArticleContent
	err          error
}

func (m *mockArticleReadDriver) FetchArticleByURLForUser(_ context.Context, _ string, _ *uuid.UUID) (*domain.ArticleContent, error) {
	return nil, nil
}

func (m *mockArticleReadDriver) FetchArticlesByURLsForUser(_ context.Context, _ []string, _ *uuid.UUID) (map[string]*domain.ArticleContent, error) {
	return nil, nil
}

func (m *mockArticleReadDriver) FetchArticleByIDForUser(_ context.Context, articleID string, userID *uuid.UUID) (*domain.ArticleContent, error) {
	m.gotArticleID = articleID
	m.gotUserID = userID
	return m.content, m.err
}

func (m *mockArticleReadDriver) FetchArticlesWithCursorForUser(_ context.Context, _ uuid.UUID, _ *time.Time, _ int) ([]*domain.Article, error) {
	return nil, nil
}

func (m *mockArticleReadDriver) FetchArticleIDsWithCursorForUser(_ context.Context, _ uuid.UUID, _ *time.Time, _ int) ([]uuid.UUID, error) {
	return nil, nil
}

func (m *mockArticleReadDriver) FetchArticlesByIDs(_ context.Context, _ []uuid.UUID) ([]*domain.Article, error) {
	return nil, nil
}

func (m *mockArticleReadDriver) FetchLatestArticleByFeedID(_ context.Context, _ uuid.UUID) (*domain.ArticleContent, error) {
	return nil, nil
}

func (m *mockArticleReadDriver) LookupArticleSource(_ context.Context, _ string, _ uuid.UUID) (domain.ArticleSource, error) {
	return domain.ArticleSource{}, nil
}

func TestArticleReadGateway_GetContentByID_PassesUserScope(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleID := "22222222-2222-4222-8222-222222222222"

	drv := &mockArticleReadDriver{
		content: &domain.ArticleContent{ID: articleID, Title: "T", Content: "C", URL: "https://example.com/a"},
	}
	gw := &ArticleReadGateway{db: drv}

	got, err := gw.GetContentByID(context.Background(), articleID, &userID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, articleID, drv.gotArticleID)
	require.NotNil(t, drv.gotUserID)
	assert.Equal(t, userID, *drv.gotUserID)
}
