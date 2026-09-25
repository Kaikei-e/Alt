package fetch_feed_gateway

import (
	"context"
	"testing"
	"time"

	"alt/orchestrator/driver/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapFeedBasic(t *testing.T) {
	feedUUID := uuid.New()
	articleID := uuid.New().String()
	ogURL := "https://example.com/image.jpg"
	now := time.Now().Truncate(time.Second)

	model := &models.Feed{
		ID:          feedUUID.String(),
		Title:       "Test Feed",
		Description: "<p>Description with <b>tags</b></p>",
		WebsiteURL:  "https://example.com/post",
		CreatedAt:   now,
		IsRead:      true,
		ArticleID:   &articleID,
		OgImageURL:  &ogURL,
	}

	item := mapFeedBasic(model)
	require.NotNil(t, item)

	// Basic list methods set only basic fields
	assert.Equal(t, "Test Feed", item.Title)
	assert.Equal(t, "https://example.com/post", item.Link)
	assert.Equal(t, now.Format(time.RFC3339), item.Published)
	assert.Equal(t, now, item.PublishedParsed)
	assert.NotContains(t, item.Description, "<p>")

	// Basic list methods do NOT set FeedID, IsRead, ArticleID, OgImageURL
	assert.Equal(t, uuid.Nil, item.FeedID)
	assert.False(t, item.IsRead)
	assert.Empty(t, item.OgImageURL)
	assert.Empty(t, item.ArticleID)
}

func TestMapFeedCursor_AllScope_IncludesIsRead(t *testing.T) {
	feedUUID := uuid.New()
	articleID := uuid.New().String()
	ogURL := "https://example.com/image.jpg"
	now := time.Now().Truncate(time.Second)

	model := &models.Feed{
		ID:          feedUUID.String(),
		Title:       "Cursor Feed",
		Description: "Simple description",
		WebsiteURL:  "https://example.com/cursor",
		CreatedAt:   now,
		IsRead:      true,
		ArticleID:   &articleID,
		OgImageURL:  &ogURL,
	}

	item, err := mapFeedCursor(model, true)
	require.NoError(t, err)
	require.NotNil(t, item)

	assert.Equal(t, feedUUID, item.FeedID)
	assert.Equal(t, "Cursor Feed", item.Title)
	assert.Equal(t, "https://example.com/cursor", item.Link)
	assert.Equal(t, now.Format(time.RFC3339), item.Published)
	assert.Equal(t, now, item.PublishedParsed)
	assert.True(t, item.IsRead)
	assert.Equal(t, articleID, item.ArticleID)
	assert.Equal(t, ogURL, item.OgImageURL)
}

func TestMapFeedCursor_Scoped_ExcludesIsRead(t *testing.T) {
	feedUUID := uuid.New()
	articleID := uuid.New().String()
	ogURL := "https://example.com/image.jpg"
	now := time.Now().Truncate(time.Second)

	model := &models.Feed{
		ID:          feedUUID.String(),
		Title:       "Unread Feed",
		Description: "Simple description",
		WebsiteURL:  "https://example.com/unread",
		CreatedAt:   now,
		IsRead:      true, // model has true, but scoped cursor does not map IsRead
		ArticleID:   &articleID,
		OgImageURL:  &ogURL,
	}

	item, err := mapFeedCursor(model, false)
	require.NoError(t, err)
	require.NotNil(t, item)

	assert.Equal(t, feedUUID, item.FeedID)
	assert.False(t, item.IsRead) // Must remain default false
	assert.Equal(t, articleID, item.ArticleID)
	assert.Equal(t, ogURL, item.OgImageURL)
}

func TestMapFeedCursor_NilOptionalFields(t *testing.T) {
	feedUUID := uuid.New()
	now := time.Now().Truncate(time.Second)

	model := &models.Feed{
		ID:         feedUUID.String(),
		Title:      "Simple Feed",
		WebsiteURL: "https://example.com/simple",
		CreatedAt:  now,
	}

	item, err := mapFeedCursor(model, true)
	require.NoError(t, err)
	require.NotNil(t, item)

	assert.Equal(t, feedUUID, item.FeedID)
	assert.Empty(t, item.ArticleID)
	assert.Empty(t, item.OgImageURL)
	assert.False(t, item.IsRead)
}

func TestMapFeedCursor_InvalidFeedID(t *testing.T) {
	now := time.Now()
	model := &models.Feed{
		ID:         "not-a-valid-uuid",
		Title:      "Invalid Feed",
		WebsiteURL: "https://example.com/invalid",
		CreatedAt:  now,
	}

	item, err := mapFeedCursor(model, false)
	require.Error(t, err)
	require.Nil(t, item)
}

func TestMapFeedCursor_EmptyFeedID(t *testing.T) {
	now := time.Now()
	model := &models.Feed{
		ID:         "",
		Title:      "Empty ID Feed",
		WebsiteURL: "https://example.com/empty",
		CreatedAt:  now,
	}

	item, err := mapFeedCursor(model, false)
	require.Error(t, err)
	require.Nil(t, item)
}

func TestParseFeedID(t *testing.T) {
	feedUUID := uuid.New()
	parsed, err := parseFeedID(feedUUID.String())
	require.NoError(t, err)
	assert.Equal(t, feedUUID, parsed)

	_, err = parseFeedID("invalid")
	require.Error(t, err)

	_, err = parseFeedID("")
	require.Error(t, err)
}

func TestDerefString(t *testing.T) {
	assert.Equal(t, "", derefString(nil))
	val := "hello"
	assert.Equal(t, "hello", derefString(&val))
}

func TestGateway_ZeroRowsSliceNilVsEmpty(t *testing.T) {
	ctx := context.Background()

	runChecks := func(t *testing.T, feeds []*models.Feed) {
		store := &feedListStoreStub{feeds: feeds, err: nil}
		gateway := NewFetchFeedsGateway(store)

		// 1. FetchFeedsList uses make([]*domain.FeedItem, 0, len(feeds)) -> non-nil empty slice
		listItems, err := gateway.FetchFeedsList(ctx)
		require.NoError(t, err)
		assert.NotNil(t, listItems)
		assert.Empty(t, listItems)

		// 2. FetchFeedsListLimit uses var feedItems []*domain.FeedItem -> nil slice
		limitItems, err := gateway.FetchFeedsListLimit(ctx, 10)
		require.NoError(t, err)
		assert.Nil(t, limitItems)

		// 3. FetchFeedsListPage uses var feedItems []*domain.FeedItem -> nil slice
		pageItems, err := gateway.FetchFeedsListPage(ctx, 1)
		require.NoError(t, err)
		assert.Nil(t, pageItems)

		// 4. FetchFeedsListCursor uses var feedItems []*domain.FeedItem -> nil slice
		cursorItems, err := gateway.FetchFeedsListCursor(ctx, nil, 10, nil)
		require.NoError(t, err)
		assert.Nil(t, cursorItems)

		// 5. FetchUnreadFeedsListCursor uses var feedItems []*domain.FeedItem -> nil slice
		unreadItems, err := gateway.FetchUnreadFeedsListCursor(ctx, nil, 10, nil)
		require.NoError(t, err)
		assert.Nil(t, unreadItems)

		// 6. FetchReadFeedsListCursor uses var feedItems []*domain.FeedItem -> nil slice
		readItems, err := gateway.FetchReadFeedsListCursor(ctx, nil, 10)
		require.NoError(t, err)
		assert.Nil(t, readItems)

		// 7. FetchFavoriteFeedsListCursor uses var feedItems []*domain.FeedItem -> nil slice
		favItems, err := gateway.FetchFavoriteFeedsListCursor(ctx, nil, 10)
		require.NoError(t, err)
		assert.Nil(t, favItems)
	}

	t.Run("nil feeds slice from store", func(t *testing.T) {
		runChecks(t, nil)
	})

	t.Run("empty feeds slice from store", func(t *testing.T) {
		runChecks(t, []*models.Feed{})
	})
}

func TestGateway_FieldPopulationsMatchHEAD(t *testing.T) {
	ctx := context.Background()
	feedUUID := uuid.New()
	articleID := uuid.New().String()
	ogURL := "https://example.com/og.png"
	now := time.Now().Truncate(time.Second)

	model := &models.Feed{
		ID:          feedUUID.String(),
		Title:       "Test Feed",
		Description: "Desc",
		WebsiteURL:  "https://example.com/post",
		CreatedAt:   now,
		IsRead:      true,
		ArticleID:   &articleID,
		OgImageURL:  &ogURL,
	}

	store := &feedListStoreStub{feeds: []*models.Feed{model}, err: nil}
	gateway := NewFetchFeedsGateway(store)

	// FetchFeedsList: basic only, no FeedID, no IsRead, no OgImageURL, no ArticleID
	listItems, err := gateway.FetchFeedsList(ctx)
	require.NoError(t, err)
	require.Len(t, listItems, 1)
	assert.Equal(t, uuid.Nil, listItems[0].FeedID)
	assert.False(t, listItems[0].IsRead)
	assert.Empty(t, listItems[0].OgImageURL)
	assert.Empty(t, listItems[0].ArticleID)

	// FetchFeedsListLimit: basic only
	limitItems, err := gateway.FetchFeedsListLimit(ctx, 10)
	require.NoError(t, err)
	require.Len(t, limitItems, 1)
	assert.Equal(t, uuid.Nil, limitItems[0].FeedID)
	assert.False(t, limitItems[0].IsRead)

	// FetchFeedsListPage: basic only
	pageItems, err := gateway.FetchFeedsListPage(ctx, 1)
	require.NoError(t, err)
	require.Len(t, pageItems, 1)
	assert.Equal(t, uuid.Nil, pageItems[0].FeedID)
	assert.False(t, pageItems[0].IsRead)

	// FetchFeedsListCursor: includes IsRead
	cursorItems, err := gateway.FetchFeedsListCursor(ctx, nil, 10, nil)
	require.NoError(t, err)
	require.Len(t, cursorItems, 1)
	assert.Equal(t, feedUUID, cursorItems[0].FeedID)
	assert.True(t, cursorItems[0].IsRead)
	assert.Equal(t, articleID, cursorItems[0].ArticleID)
	assert.Equal(t, ogURL, cursorItems[0].OgImageURL)

	// FetchUnreadFeedsListCursor: excludes IsRead
	unreadItems, err := gateway.FetchUnreadFeedsListCursor(ctx, nil, 10, nil)
	require.NoError(t, err)
	require.Len(t, unreadItems, 1)
	assert.Equal(t, feedUUID, unreadItems[0].FeedID)
	assert.False(t, unreadItems[0].IsRead)
	assert.Equal(t, articleID, unreadItems[0].ArticleID)
	assert.Equal(t, ogURL, unreadItems[0].OgImageURL)

	// FetchReadFeedsListCursor: excludes IsRead
	readItems, err := gateway.FetchReadFeedsListCursor(ctx, nil, 10)
	require.NoError(t, err)
	require.Len(t, readItems, 1)
	assert.Equal(t, feedUUID, readItems[0].FeedID)
	assert.False(t, readItems[0].IsRead)

	// FetchFavoriteFeedsListCursor: excludes IsRead
	favItems, err := gateway.FetchFavoriteFeedsListCursor(ctx, nil, 10)
	require.NoError(t, err)
	require.Len(t, favItems, 1)
	assert.Equal(t, feedUUID, favItems[0].FeedID)
	assert.False(t, favItems[0].IsRead)
}
