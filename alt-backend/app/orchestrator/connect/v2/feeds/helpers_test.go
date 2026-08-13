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
	"alt/orchestrator/usecase/cached_feed_list_usecase"
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
