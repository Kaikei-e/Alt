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

type mockOgImageDriver struct {
	gotArticleIDs []string
	gotUser       *uuid.UUID
	urlsByUser    map[uuid.UUID]map[string]string
}

func (m *mockOgImageDriver) SaveArticleHead(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockOgImageDriver) FetchArticleHeadByArticleID(_ context.Context, _ string) (*domain.ArticleHead, error) {
	return nil, nil
}

func (m *mockOgImageDriver) FetchOgImageURLsByArticleIDsForUser(_ context.Context, articleIDs []string, userID *uuid.UUID) (map[string]string, error) {
	m.gotArticleIDs, m.gotUser = articleIDs, userID
	if userID == nil {
		return nil, nil
	}
	return m.urlsByUser[*userID], nil
}

func (m *mockOgImageDriver) FetchUnwarmedOgImageURLs(_ context.Context, _ int) ([]string, error) {
	return nil, nil
}

func (m *mockOgImageDriver) CleanupExpiredArticleHeads(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

func (m *mockOgImageDriver) FetchFeedOgImageTargets(_ context.Context, _ []string) ([]domain.FeedOgImageTarget, error) {
	return nil, nil
}

func (m *mockOgImageDriver) SaveFeedOgImage(_ context.Context, _, _ string, _ domain.OgImageRefusal, _ time.Duration) error {
	return nil
}

func (m *mockOgImageDriver) CleanupExpiredFeedOgImages(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

func TestOgImageGateway_BatchGetOgImageURLs_ForwardsUserScope(t *testing.T) {
	userA := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	userB := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	articleID := "22222222-2222-4222-8222-222222222222"

	mock := &mockOgImageDriver{
		urlsByUser: map[uuid.UUID]map[string]string{
			userA: {articleID: "https://cdn.example.com/private-a.png"},
		},
	}
	gw := &OgImageGateway{db: mock}

	urlsA, err := gw.BatchGetOgImageURLs(context.Background(), []string{articleID}, &userA)
	require.NoError(t, err)
	assert.Equal(t, []string{articleID}, mock.gotArticleIDs)
	require.NotNil(t, mock.gotUser)
	assert.Equal(t, userA, *mock.gotUser)
	assert.Equal(t, map[string]string{articleID: "https://cdn.example.com/private-a.png"}, urlsA)

	urlsB, err := gw.BatchGetOgImageURLs(context.Background(), []string{articleID}, &userB)
	require.NoError(t, err)
	require.NotNil(t, mock.gotUser)
	assert.Equal(t, userB, *mock.gotUser)
	assert.Empty(t, urlsB)
}
