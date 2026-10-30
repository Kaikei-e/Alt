package rest_feeds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/orchestrator/usecase/cached_feed_list_usecase"
	"alt/orchestrator/usecase/fetch_feed_usecase"
	"alt/utils/logger"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeriveNextCursorFromFeeds_KeepsSubSecondPrecision pins the pagination
// boundary: created_at is microsecond precision and one harvester transaction
// stamps a whole batch inside the same second, so a cursor that only names the
// second is fed back into `created_at < $1` and drops the rest of that second.
func TestDeriveNextCursorFromFeeds_KeepsSubSecondPrecision(t *testing.T) {
	ts := time.Date(2026, time.March, 2, 10, 0, 0, 123456000, time.UTC)

	cursor, ok := DeriveNextCursorFromFeeds([]*domain.FeedItem{{PublishedParsed: ts}})
	if !ok {
		t.Fatalf("expected cursor to be derived")
	}

	parsed, err := time.Parse(time.RFC3339, cursor)
	if err != nil {
		t.Fatalf("cursor %q is not parseable by the cursor parse side: %v", cursor, err)
	}
	if !parsed.Equal(ts) {
		t.Fatalf("cursor lost precision: %s became %s", ts.Format(time.RFC3339Nano), parsed.Format(time.RFC3339Nano))
	}
}

func TestDeriveNextCursorFromFeeds_FallsBackToPublishedString(t *testing.T) {
	published := "2026-03-02T10:00:00.123456Z"

	cursor, ok := DeriveNextCursorFromFeeds([]*domain.FeedItem{{Published: published}})
	if !ok {
		t.Fatalf("expected cursor to be derived from the published string")
	}
	if cursor != published {
		t.Fatalf("expected cursor %s, got %s", published, cursor)
	}
}

func TestDeriveNextCursorFromFeeds(t *testing.T) {
	t.Run("returns false when feeds slice is empty", func(t *testing.T) {
		if cursor, ok := DeriveNextCursorFromFeeds(nil); ok || cursor != "" {
			t.Fatalf("expected no cursor, got %v (ok=%v)", cursor, ok)
		}
	})

	t.Run("uses PublishedParsed when available", func(t *testing.T) {
		ts := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
		cursor, ok := DeriveNextCursorFromFeeds([]*domain.FeedItem{
			{
				Title:           "foo",
				PublishedParsed: ts,
			},
		})
		if !ok {
			t.Fatalf("expected cursor to be derived")
		}
		if cursor != ts.Format(time.RFC3339Nano) {
			t.Fatalf("unexpected cursor %s", cursor)
		}
	})

	t.Run("falls back to parsing Published string", func(t *testing.T) {
		published := "2024-02-01T00:00:00Z"
		cursor, ok := DeriveNextCursorFromFeeds([]*domain.FeedItem{
			{
				Title:     "bar",
				Published: published,
			},
		})
		if !ok {
			t.Fatalf("expected cursor to be derived from string")
		}
		expected, _ := time.Parse(time.RFC3339, published)
		if cursor != expected.Format(time.RFC3339Nano) {
			t.Fatalf("expected cursor %s, got %s", expected.Format(time.RFC3339Nano), cursor)
		}
	})

	t.Run("returns false when published string is invalid", func(t *testing.T) {
		cursor, ok := DeriveNextCursorFromFeeds([]*domain.FeedItem{
			{
				Published: "invalid",
			},
		})
		if ok || cursor != "" {
			t.Fatalf("expected derive failure, got %v (ok=%v)", cursor, ok)
		}
	})
}

type stubFeedCursorPort struct{}

func (stubFeedCursorPort) FetchUnreadFeedsListCursor(_ context.Context, _ *time.Time, _ int, _ []uuid.UUID) ([]*domain.FeedItem, error) {
	return nil, nil
}
func (stubFeedCursorPort) FetchReadFeedsListCursor(_ context.Context, _ *time.Time, _ int) ([]*domain.FeedItem, error) {
	return nil, nil
}
func (stubFeedCursorPort) FetchFavoriteFeedsListCursor(_ context.Context, _ *time.Time, _ int) ([]*domain.FeedItem, error) {
	return nil, nil
}

func TestCursorFeedHandlers_SetPrivateNoCacheHeader(t *testing.T) {
	logger.InitLogger()
	stub := stubFeedCursorPort{}
	container := &di.ApplicationComponents{
		CachedFeedListUsecase:               cached_feed_list_usecase.NewCachedFeedListUsecase(stub, stub, nil),
		FetchReadFeedsListCursorUsecase:     fetch_feed_usecase.NewFetchReadFeedsListCursorUsecase(stub),
		FetchFavoriteFeedsListCursorUsecase: fetch_feed_usecase.NewFetchFavoriteFeedsListCursorUsecase(stub),
	}

	e := echo.New()
	tests := []struct {
		name    string
		target  string
		handler echo.HandlerFunc
	}{
		{
			name:    "unread feeds first page",
			target:  "/v1/feeds/fetch/cursor",
			handler: RestHandleFetchUnreadFeedsCursor(container),
		},
		{
			name:    "unread feeds subsequent page",
			target:  "/v1/feeds/fetch/cursor?cursor=2026-03-02T10:00:00Z",
			handler: RestHandleFetchUnreadFeedsCursor(container),
		},
		{
			name:    "read feeds cursor",
			target:  "/v1/feeds/fetch/viewed/cursor",
			handler: RestHandleFetchReadFeedsCursor(container),
		},
		{
			name:    "favorite feeds cursor",
			target:  "/v1/feeds/fetch/favorites/cursor",
			handler: RestHandleFetchFavoriteFeedsCursor(container),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := tt.handler(c)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
		})
	}
}

func (stubFeedCursorPort) FetchSingleFeed(_ context.Context) (*domain.RSSFeed, error) {
	return &domain.RSSFeed{Title: "stub"}, nil
}
func (stubFeedCursorPort) FetchFeedsList(_ context.Context) ([]*domain.FeedItem, error) {
	return nil, nil
}
func (stubFeedCursorPort) FetchFeedsListLimit(_ context.Context, _ int) ([]*domain.FeedItem, error) {
	return nil, nil
}
func (stubFeedCursorPort) FetchFeedsListPage(_ context.Context, _ int) ([]*domain.FeedItem, error) {
	return nil, nil
}

func TestFeedFetchHandlers_SetPrivateNoCacheHeader(t *testing.T) {
	logger.InitLogger()
	stub := stubFeedCursorPort{}
	cfg := &config.Config{}
	container := &di.ApplicationComponents{
		FetchSingleFeedUsecase: fetch_feed_usecase.NewFetchSingleFeedUsecase(stub),
		FetchFeedsListUsecase:  fetch_feed_usecase.NewFetchFeedsListUsecase(stub),
	}

	e := echo.New()
	tests := []struct {
		name       string
		target     string
		paramKey   string
		paramValue string
		handler    echo.HandlerFunc
	}{
		{
			name:    "single feed",
			target:  "/v1/feeds/fetch/single",
			handler: RestHandleFetchSingleFeed(container, cfg),
		},
		{
			name:    "feeds list",
			target:  "/v1/feeds/fetch/list",
			handler: RestHandleFetchFeedsList(container, cfg),
		},
		{
			name:       "feeds limit",
			target:     "/v1/feeds/fetch/limit/10",
			paramKey:   "limit",
			paramValue: "10",
			handler:    RestHandleFetchFeedsLimit(container, cfg),
		},
		{
			name:       "feeds page",
			target:     "/v1/feeds/fetch/page/1",
			paramKey:   "page",
			paramValue: "1",
			handler:    RestHandleFetchFeedsPage(container),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tt.paramKey != "" {
				c.SetParamNames(tt.paramKey)
				c.SetParamValues(tt.paramValue)
			}

			err := tt.handler(c)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
		})
	}
}
