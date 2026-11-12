package datahub_capability_gateway

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockReadStateDriver struct {
	lastUserID  uuid.UUID
	lastSince   *time.Time
	readFeedIDs map[uuid.UUID]bool
	err         error
}

func (m *mockReadStateDriver) UpdateFeedStatus(_ context.Context, _ url.URL, _ uuid.UUID) error {
	return nil
}
func (m *mockReadStateDriver) MarkArticleAsRead(_ context.Context, _ url.URL, _ uuid.UUID) error {
	return nil
}
func (m *mockReadStateDriver) GetReadFeedIDs(_ context.Context, _ uuid.UUID, _ []uuid.UUID) (map[uuid.UUID]bool, error) {
	return nil, nil
}
func (m *mockReadStateDriver) GetAllReadFeedIDs(_ context.Context, userID uuid.UUID, since *time.Time) (map[uuid.UUID]bool, error) {
	m.lastUserID = userID
	m.lastSince = since
	if m.err != nil {
		return nil, m.err
	}
	return m.readFeedIDs, nil
}
func (m *mockReadStateDriver) GetUserSubscriptions(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *mockReadStateDriver) FetchSubscriptions(_ context.Context, _ uuid.UUID) ([]*domain.FeedSource, error) {
	return nil, nil
}
func (m *mockReadStateDriver) InsertSubscription(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}
func (m *mockReadStateDriver) DeleteSubscription(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}
func (m *mockReadStateDriver) RegisterFavoriteFeed(_ context.Context, _ string, _ uuid.UUID) error {
	return nil
}
func (m *mockReadStateDriver) RemoveFavoriteFeed(_ context.Context, _ string, _ uuid.UUID) error {
	return nil
}

func TestReadStateGateway_AllReadFeedIDs_WithoutSince(t *testing.T) {
	userID := uuid.New()
	feedID1 := uuid.New()
	feedID2 := uuid.New()

	mock := &mockReadStateDriver{
		readFeedIDs: map[uuid.UUID]bool{
			feedID1: true,
			feedID2: true,
		},
	}
	gw := &ReadStateGateway{db: mock}

	ids, err := gw.AllReadFeedIDs(context.Background(), userID, nil)
	require.NoError(t, err)
	assert.Equal(t, userID, mock.lastUserID)
	assert.Nil(t, mock.lastSince)
	assert.ElementsMatch(t, []uuid.UUID{feedID1, feedID2}, ids)
}

func TestReadStateGateway_AllReadFeedIDs_WithSince(t *testing.T) {
	userID := uuid.New()
	since := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	feedID1 := uuid.New()

	mock := &mockReadStateDriver{
		readFeedIDs: map[uuid.UUID]bool{
			feedID1: true,
		},
	}
	gw := &ReadStateGateway{db: mock}

	ids, err := gw.AllReadFeedIDs(context.Background(), userID, &since)
	require.NoError(t, err)
	assert.Equal(t, userID, mock.lastUserID)
	require.NotNil(t, mock.lastSince)
	assert.Equal(t, since, *mock.lastSince)
	assert.Equal(t, []uuid.UUID{feedID1}, ids)
}

func TestReadStateGateway_AllReadFeedIDs_DriverError(t *testing.T) {
	userID := uuid.New()
	mock := &mockReadStateDriver{
		err: errors.New("db connection failure"),
	}
	gw := &ReadStateGateway{db: mock}

	ids, err := gw.AllReadFeedIDs(context.Background(), userID, nil)
	require.Error(t, err)
	assert.Nil(t, ids)
	assert.Contains(t, err.Error(), "get all read feed ids for user")
}

type mockTagReadDriver struct {
	cloud        []*domain.TagCloudItem
	gotCloudUser uuid.UUID
	gotCloudLim  int
	gotArticleID string
	gotUser      *uuid.UUID
	tagsByUser   map[uuid.UUID][]*domain.FeedTag
}

func (m *mockTagReadDriver) FetchArticleTagsForUser(_ context.Context, articleID string, userID *uuid.UUID) ([]*domain.FeedTag, error) {
	m.gotArticleID, m.gotUser = articleID, userID
	if userID == nil {
		return nil, nil
	}
	return m.tagsByUser[*userID], nil
}

func (m *mockTagReadDriver) FetchFeedTags(_ context.Context, _ string, _ *time.Time, _ int) ([]*domain.FeedTag, error) {
	return nil, nil
}

func (m *mockTagReadDriver) FetchTagCooccurrences(_ context.Context, _ []string) ([]*domain.TagCooccurrence, error) {
	return nil, nil
}

func (m *mockTagReadDriver) SearchTagsByPrefix(_ context.Context, _ string, _ uuid.UUID, _ int) ([]domain.GlobalTagHit, error) {
	return nil, nil
}

func (m *mockTagReadDriver) FetchTagCloudForUser(_ context.Context, userID uuid.UUID, limit int) ([]*domain.TagCloudItem, error) {
	m.gotCloudUser, m.gotCloudLim = userID, limit
	return m.cloud, nil
}

func (m *mockTagReadDriver) FetchTagArticleCounts(_ context.Context, _ uuid.UUID, _ time.Time) ([]domain.TagArticleCount, error) {
	return nil, nil
}

func TestTagReadGateway_ArticleTags_ForwardsUserScope(t *testing.T) {
	userA := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	userB := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	articleID := "22222222-2222-4222-8222-222222222222"

	mock := &mockTagReadDriver{
		tagsByUser: map[uuid.UUID][]*domain.FeedTag{
			userA: {{ID: "t1", TagName: "PrivateTag"}},
		},
	}
	gw := &TagReadGateway{db: mock}

	tagsA, err := gw.ArticleTags(context.Background(), articleID, &userA)
	require.NoError(t, err)
	assert.Equal(t, articleID, mock.gotArticleID)
	require.NotNil(t, mock.gotUser)
	assert.Equal(t, userA, *mock.gotUser)
	require.Len(t, tagsA, 1)
	assert.Equal(t, "PrivateTag", tagsA[0].TagName)

	tagsB, err := gw.ArticleTags(context.Background(), articleID, &userB)
	require.NoError(t, err)
	require.NotNil(t, mock.gotUser)
	assert.Equal(t, userB, *mock.gotUser)
	assert.Empty(t, tagsB)
}

func TestTagReadGateway_TagCloud_ForwardsUserScope(t *testing.T) {
	user := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	mock := &mockTagReadDriver{cloud: []*domain.TagCloudItem{{TagName: "mine", ArticleCount: 2}}}
	gw := &TagReadGateway{db: mock}

	items, err := gw.TagCloud(context.Background(), user, 25)
	require.NoError(t, err)
	assert.Equal(t, user, mock.gotCloudUser)
	assert.Equal(t, 25, mock.gotCloudLim)
	require.Len(t, items, 1)
	assert.Equal(t, "mine", items[0].TagName)
}
