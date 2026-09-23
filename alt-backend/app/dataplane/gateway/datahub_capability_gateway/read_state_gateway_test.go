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
