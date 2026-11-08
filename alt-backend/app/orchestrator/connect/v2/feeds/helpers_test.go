package feeds

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/orchestrator/driver/models"
	"alt/orchestrator/gateway/fetch_feed_gateway"
	"alt/orchestrator/usecase/cached_feed_list_usecase"
	"alt/orchestrator/usecase/fetch_feed_usecase"
	"alt/orchestrator/usecase/resolve_article_usecase"
	"alt/utils/logger"
)

func TestConvertFeedsToProto_IDPriority(t *testing.T) {
	tests := []struct {
		name       string
		feed       *domain.FeedItem
		expectedID string
		expectUUID bool // when true, assert valid UUID format instead of exact match
	}{
		{
			name: "uses ArticleID when set",
			feed: &domain.FeedItem{
				Link:      "https://example.com/feed",
				ArticleID: "article-456",
			},
			expectedID: "article-456",
		},
		{
			name: "generates UUID when ArticleID is empty",
			feed: &domain.FeedItem{
				Link: "https://example.com/feed",
			},
			expectUUID: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertFeedsToProto([]*domain.FeedItem{tt.feed})
			assert.Len(t, result, 1)
			if tt.expectUUID {
				// UUID v4 format: 8-4-4-4-12 hex chars
				assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, result[0].Id)
			} else {
				assert.Equal(t, tt.expectedID, result[0].Id)
			}
		})
	}
}

func TestConvertFeedsToProto_DuplicateLinksUniqueIDs(t *testing.T) {
	// Simulates the favorites scenario: multiple feed items from the same source
	// have the same Link and no ArticleID — UUID fallback guarantees unique keys.
	feeds := []*domain.FeedItem{
		{Link: "https://example.com/feed", Title: "Article A"},
		{Link: "https://example.com/feed", Title: "Article B"},
		{Link: "https://example.com/feed", Title: "Article C"},
	}

	result := convertFeedsToProto(feeds)
	assert.Len(t, result, 3)

	ids := make(map[string]bool)
	for _, item := range result {
		assert.False(t, ids[item.Id], "duplicate ID found: %s", item.Id)
		ids[item.Id] = true
	}
}

// fakeUnreadFeedCursorPort records the cursor the handler passed down, so the
// page-2 request can be compared against the timestamp page 1 handed out.
type fakeUnreadFeedCursorPort struct {
	feeds      []*domain.FeedItem
	lastCursor *time.Time
}

func (f *fakeUnreadFeedCursorPort) FetchUnreadFeedsListCursor(_ context.Context, cursor *time.Time, _ int, _ []uuid.UUID) ([]*domain.FeedItem, error) {
	f.lastCursor = cursor
	return f.feeds, nil
}

// TestGetUnreadFeeds_CursorRoundTripKeepsSubSecondPrecision pins the pagination
// boundary: feeds.created_at is microsecond precision and one harvester
// transaction stamps a whole batch inside the same second, so a cursor that
// only names the second is fed back into `created_at < $1` and drops the rest
// of that second from the timeline for good.
func TestGetUnreadFeeds_CursorRoundTripKeepsSubSecondPrecision(t *testing.T) {
	logger.InitLogger()

	firstPageTail := time.Date(2026, time.March, 2, 10, 0, 0, 123456000, time.UTC)
	sameSecond := firstPageTail.Add(-500 * time.Microsecond)

	port := &fakeUnreadFeedCursorPort{feeds: []*domain.FeedItem{
		{
			Title:           "first",
			Link:            "https://example.com/1",
			Published:       firstPageTail.Format(time.RFC3339Nano),
			PublishedParsed: firstPageTail,
		},
		{
			Title:           "second",
			Link:            "https://example.com/2",
			Published:       sameSecond.Format(time.RFC3339Nano),
			PublishedParsed: sameSecond,
		},
	}}

	handler := NewHandler(FeedHandlerDeps{
		CachedFeedList: cached_feed_list_usecase.NewCachedFeedListUsecase(nil, port, nil),
		ResolveArticle: resolve_article_usecase.New(nil, nil),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	page1, err := handler.GetUnreadFeeds(ctx, connect.NewRequest(&feedsv2.GetUnreadFeedsRequest{Limit: 1}))
	require.NoError(t, err)
	require.True(t, page1.Msg.HasMore)
	require.NotNil(t, page1.Msg.NextCursor)

	_, err = handler.GetUnreadFeeds(ctx, connect.NewRequest(&feedsv2.GetUnreadFeedsRequest{
		Limit:  1,
		Cursor: page1.Msg.NextCursor,
	}))
	require.NoError(t, err)
	require.NotNil(t, port.lastCursor)

	assert.True(t, port.lastCursor.Equal(firstPageTail),
		"cursor %q became %q, so every row between them is skipped",
		firstPageTail.Format(time.RFC3339Nano), port.lastCursor.Format(time.RFC3339Nano))
}

type cursorFilteringStoreStub struct {
	rows []*models.Feed
}

func (s *cursorFilteringStoreStub) FetchFeedsList(context.Context) ([]*models.Feed, error) {
	return nil, nil
}
func (s *cursorFilteringStoreStub) FetchFeedsListLimit(context.Context, int) ([]*models.Feed, error) {
	return nil, nil
}
func (s *cursorFilteringStoreStub) FetchUnreadFeedsListPage(context.Context, int) ([]*models.Feed, error) {
	return nil, nil
}
func (s *cursorFilteringStoreStub) FetchAllFeedsListCursor(context.Context, *time.Time, int, []uuid.UUID) ([]*models.Feed, error) {
	return nil, nil
}
func (s *cursorFilteringStoreStub) FetchUnreadFeedsListCursor(context.Context, *time.Time, int, []uuid.UUID) ([]*models.Feed, error) {
	return nil, nil
}
func (s *cursorFilteringStoreStub) FetchReadFeedsListCursor(_ context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	return s.filterBySortKey(cursor, limit), nil
}
func (s *cursorFilteringStoreStub) FetchFavoriteFeedsListCursor(_ context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	return s.filterBySortKey(cursor, limit), nil
}

func (s *cursorFilteringStoreStub) filterBySortKey(cursor *time.Time, limit int) []*models.Feed {
	var out []*models.Feed
	for _, r := range s.rows {
		if cursor == nil || r.UpdatedAt.Before(*cursor) {
			out = append(out, r)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

func TestGetReadFeeds_CursorUsesReadAtAndReturnsPage2(t *testing.T) {
	logger.InitLogger()

	t1 := time.Date(2026, time.March, 1, 10, 0, 0, 111000000, time.UTC)
	t2 := time.Date(2026, time.March, 2, 10, 0, 0, 222000000, time.UTC)
	t8 := time.Date(2026, time.March, 8, 10, 0, 0, 888123456, time.UTC)
	t9 := time.Date(2026, time.March, 9, 10, 0, 0, 999654321, time.UTC)

	store := &cursorFilteringStoreStub{
		rows: []*models.Feed{
			{
				ID:         uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String(),
				Title:      "Feed A",
				WebsiteURL: "https://example.com/a",
				CreatedAt:  t1,
				UpdatedAt:  t9,
			},
			{
				ID:         uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb").String(),
				Title:      "Feed B",
				WebsiteURL: "https://example.com/b",
				CreatedAt:  t2,
				UpdatedAt:  t8,
			},
		},
	}

	gw := fetch_feed_gateway.NewFetchFeedsGateway(store)
	handler := NewHandler(FeedHandlerDeps{
		FetchReadFeedsCursor: fetch_feed_usecase.NewFetchReadFeedsListCursorUsecase(gw),
		ResolveArticle:       resolve_article_usecase.New(nil, nil),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	page1, err := handler.GetReadFeeds(ctx, connect.NewRequest(&feedsv2.GetReadFeedsRequest{Limit: 1}))
	require.NoError(t, err)
	require.Len(t, page1.Msg.Data, 1)
	assert.Equal(t, "Feed A", page1.Msg.Data[0].Title)
	require.NotNil(t, page1.Msg.NextCursor)
	assert.Equal(t, t9.Format(time.RFC3339Nano), *page1.Msg.NextCursor)

	page2, err := handler.GetReadFeeds(ctx, connect.NewRequest(&feedsv2.GetReadFeedsRequest{
		Limit:  1,
		Cursor: page1.Msg.NextCursor,
	}))
	require.NoError(t, err)
	require.Len(t, page2.Msg.Data, 1)
	assert.Equal(t, "Feed B", page2.Msg.Data[0].Title)
}

func TestGetFavoriteFeeds_CursorUsesFavoriteCreatedAtAndReturnsPage2(t *testing.T) {
	logger.InitLogger()

	t1 := time.Date(2026, time.March, 1, 10, 0, 0, 111000000, time.UTC)
	t2 := time.Date(2026, time.March, 2, 10, 0, 0, 222000000, time.UTC)
	t8 := time.Date(2026, time.March, 8, 10, 0, 0, 888123456, time.UTC)
	t9 := time.Date(2026, time.March, 9, 10, 0, 0, 999654321, time.UTC)

	store := &cursorFilteringStoreStub{
		rows: []*models.Feed{
			{
				ID:         uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String(),
				Title:      "Feed A",
				WebsiteURL: "https://example.com/a",
				CreatedAt:  t1,
				UpdatedAt:  t9,
			},
			{
				ID:         uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb").String(),
				Title:      "Feed B",
				WebsiteURL: "https://example.com/b",
				CreatedAt:  t2,
				UpdatedAt:  t8,
			},
		},
	}

	gw := fetch_feed_gateway.NewFetchFeedsGateway(store)
	handler := NewHandler(FeedHandlerDeps{
		FetchFavoriteFeedsCursor: fetch_feed_usecase.NewFetchFavoriteFeedsListCursorUsecase(gw),
		ResolveArticle:           resolve_article_usecase.New(nil, nil),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	page1, err := handler.GetFavoriteFeeds(ctx, connect.NewRequest(&feedsv2.GetFavoriteFeedsRequest{Limit: 1}))
	require.NoError(t, err)
	require.Len(t, page1.Msg.Data, 1)
	assert.Equal(t, "Feed A", page1.Msg.Data[0].Title)
	require.NotNil(t, page1.Msg.NextCursor)
	assert.Equal(t, t9.Format(time.RFC3339Nano), *page1.Msg.NextCursor)

	page2, err := handler.GetFavoriteFeeds(ctx, connect.NewRequest(&feedsv2.GetFavoriteFeedsRequest{
		Limit:  1,
		Cursor: page1.Msg.NextCursor,
	}))
	require.NoError(t, err)
	require.Len(t, page2.Msg.Data, 1)
	assert.Equal(t, "Feed B", page2.Msg.Data[0].Title)
}

type fakeFeedTagStore struct {
	tags       []*domain.FeedTag
	lastFeedID string
	lastCursor *time.Time
	called     bool
}

func (f *fakeFeedTagStore) FetchFeedTags(_ context.Context, feedID string, cursor *time.Time, _ int) ([]*domain.FeedTag, error) {
	f.called = true
	f.lastFeedID = feedID
	f.lastCursor = cursor
	return f.tags, nil
}

type missingCursorAtFeedPortStub struct {
	items []*domain.FeedItem
}

func (s *missingCursorAtFeedPortStub) FetchReadFeedsListCursor(_ context.Context, _ *time.Time, _ int) ([]*domain.FeedItem, error) {
	return s.items, nil
}

func (s *missingCursorAtFeedPortStub) FetchFavoriteFeedsListCursor(_ context.Context, _ *time.Time, _ int) ([]*domain.FeedItem, error) {
	return s.items, nil
}

func TestGetReadAndFavoriteFeeds_MissingCursorAtReturnsError(t *testing.T) {
	logger.InitLogger()

	createdAt := time.Date(2026, time.March, 1, 10, 0, 0, 111000000, time.UTC)
	portStub := &missingCursorAtFeedPortStub{
		items: []*domain.FeedItem{{
			Title:           "Feed Missing CursorAt",
			Link:            "https://example.com/a",
			Published:       createdAt.Format(time.RFC3339),
			PublishedParsed: createdAt,
		}},
	}

	handler := NewHandler(FeedHandlerDeps{
		FetchReadFeedsCursor:     fetch_feed_usecase.NewFetchReadFeedsListCursorUsecase(portStub),
		FetchFavoriteFeedsCursor: fetch_feed_usecase.NewFetchFavoriteFeedsListCursorUsecase(portStub),
		ResolveArticle:           resolve_article_usecase.New(nil, nil),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	_, err := handler.GetReadFeeds(ctx, connect.NewRequest(&feedsv2.GetReadFeedsRequest{Limit: 1}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))

	_, err = handler.GetFavoriteFeeds(ctx, connect.NewRequest(&feedsv2.GetFavoriteFeedsRequest{Limit: 1}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestGetFeedTags_NormalizesUUID(t *testing.T) {
	logger.InitLogger()

	store := &fakeFeedTagStore{}
	handler := NewHandler(FeedHandlerDeps{
		FeedTagStore:   store,
		ResolveArticle: resolve_article_usecase.New(nil, nil),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	_, err := handler.GetFeedTags(ctx, connect.NewRequest(&feedsv2.GetFeedTagsRequest{
		FeedId: "urn:uuid:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
	}))
	require.NoError(t, err)
	assert.True(t, store.called)
	assert.Equal(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", store.lastFeedID)
}

func TestGetFeedTags_RejectsMalformedUUID(t *testing.T) {
	logger.InitLogger()

	store := &fakeFeedTagStore{}
	handler := NewHandler(FeedHandlerDeps{
		FeedTagStore:   store,
		ResolveArticle: resolve_article_usecase.New(nil, nil),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	_, err := handler.GetFeedTags(ctx, connect.NewRequest(&feedsv2.GetFeedTagsRequest{
		FeedId: "not-a-uuid",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.False(t, store.called)
}

func TestGetFeedTags_CursorRoundTripKeepsSubSecondPrecision(t *testing.T) {
	logger.InitLogger()

	ts := time.Date(2026, time.March, 2, 10, 0, 0, 123456000, time.UTC)
	store := &fakeFeedTagStore{
		tags: []*domain.FeedTag{
			{
				ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111").String(),
				TagName:   "golang",
				CreatedAt: ts,
			},
		},
	}

	handler := NewHandler(FeedHandlerDeps{
		FeedTagStore:   store,
		ResolveArticle: resolve_article_usecase.New(nil, nil),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()
	feedID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()

	page1, err := handler.GetFeedTags(ctx, connect.NewRequest(&feedsv2.GetFeedTagsRequest{
		FeedId: feedID,
		Limit:  1,
	}))
	require.NoError(t, err)
	require.Len(t, page1.Msg.Tags, 1)
	nextCursor := page1.Msg.Tags[0].CreatedAt

	_, err = handler.GetFeedTags(ctx, connect.NewRequest(&feedsv2.GetFeedTagsRequest{
		FeedId: feedID,
		Limit:  1,
		Cursor: nextCursor,
	}))
	require.NoError(t, err)
	require.NotNil(t, store.lastCursor)
	assert.True(t, store.lastCursor.Equal(ts),
		"cursor %q became %q", ts.Format(time.RFC3339Nano), store.lastCursor.Format(time.RFC3339Nano))

	wholeSec := time.Date(2026, time.March, 2, 10, 0, 0, 0, time.UTC)
	_, err = handler.GetFeedTags(ctx, connect.NewRequest(&feedsv2.GetFeedTagsRequest{
		FeedId: feedID,
		Limit:  1,
		Cursor: wholeSec.Format(time.RFC3339),
	}))
	require.NoError(t, err)
	require.NotNil(t, store.lastCursor)
	assert.True(t, store.lastCursor.Equal(wholeSec))
}
