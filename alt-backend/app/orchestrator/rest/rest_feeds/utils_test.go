package rest_feeds

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/orchestrator/driver/models"
	"alt/orchestrator/gateway/fetch_feed_gateway"
	"alt/orchestrator/usecase/cached_feed_list_usecase"
	"alt/orchestrator/usecase/fetch_feed_tags_by_id_usecase"
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

type restCursorFilteringStoreStub struct {
	rows []*models.Feed
}

func (s *restCursorFilteringStoreStub) FetchFeedsList(context.Context) ([]*models.Feed, error) {
	return nil, nil
}
func (s *restCursorFilteringStoreStub) FetchFeedsListLimit(context.Context, int) ([]*models.Feed, error) {
	return nil, nil
}
func (s *restCursorFilteringStoreStub) FetchUnreadFeedsListPage(context.Context, int) ([]*models.Feed, error) {
	return nil, nil
}
func (s *restCursorFilteringStoreStub) FetchAllFeedsListCursor(context.Context, *time.Time, int, []uuid.UUID) ([]*models.Feed, error) {
	return nil, nil
}
func (s *restCursorFilteringStoreStub) FetchUnreadFeedsListCursor(context.Context, *time.Time, int, []uuid.UUID) ([]*models.Feed, error) {
	return nil, nil
}
func (s *restCursorFilteringStoreStub) FetchReadFeedsListCursor(_ context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	return s.filterBySortKey(cursor, limit), nil
}
func (s *restCursorFilteringStoreStub) FetchFavoriteFeedsListCursor(_ context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	return s.filterBySortKey(cursor, limit), nil
}

func (s *restCursorFilteringStoreStub) filterBySortKey(cursor *time.Time, limit int) []*models.Feed {
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

func TestRestHandleFetchReadAndFavoriteFeedsCursor_UsesSortKeyTime(t *testing.T) {
	logger.InitLogger()

	t1 := time.Date(2026, time.March, 1, 10, 0, 0, 111000000, time.UTC)
	t2 := time.Date(2026, time.March, 2, 10, 0, 0, 222000000, time.UTC)
	t8 := time.Date(2026, time.March, 8, 10, 0, 0, 888123456, time.UTC)
	t9 := time.Date(2026, time.March, 9, 10, 0, 0, 999654321, time.UTC)

	store := &restCursorFilteringStoreStub{
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
	container := &di.ApplicationComponents{
		FetchReadFeedsListCursorUsecase:     fetch_feed_usecase.NewFetchReadFeedsListCursorUsecase(gw),
		FetchFavoriteFeedsListCursorUsecase: fetch_feed_usecase.NewFetchFavoriteFeedsListCursorUsecase(gw),
	}

	e := echo.New()
	tests := []struct {
		name    string
		baseURL string
		handler echo.HandlerFunc
	}{
		{
			name:    "read feeds cursor",
			baseURL: "/v1/feeds/fetch/viewed/cursor",
			handler: RestHandleFetchReadFeedsCursor(container),
		},
		{
			name:    "favorite feeds cursor",
			baseURL: "/v1/feeds/fetch/favorites/cursor",
			handler: RestHandleFetchFavoriteFeedsCursor(container),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req1 := httptest.NewRequest(http.MethodGet, tt.baseURL+"?limit=1", nil)
			rec1 := httptest.NewRecorder()
			c1 := e.NewContext(req1, rec1)

			require.NoError(t, tt.handler(c1))
			require.Equal(t, http.StatusOK, rec1.Code)

			var body1 struct {
				Data       []map[string]any `json:"data"`
				NextCursor string           `json:"next_cursor"`
			}
			require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &body1))
			require.Len(t, body1.Data, 1)
			assert.Equal(t, "Feed A", body1.Data[0]["title"])
			assert.Equal(t, t9.Format(time.RFC3339Nano), body1.NextCursor)

			req2 := httptest.NewRequest(http.MethodGet, tt.baseURL+"?limit=1&cursor="+url.QueryEscape(body1.NextCursor), nil)
			rec2 := httptest.NewRecorder()
			c2 := e.NewContext(req2, rec2)

			require.NoError(t, tt.handler(c2))
			require.Equal(t, http.StatusOK, rec2.Code)

			var body2 struct {
				Data []map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &body2))
			require.Len(t, body2.Data, 1)
			assert.Equal(t, "Feed B", body2.Data[0]["title"])
		})
	}
}

type stubFetchFeedTagsPort struct {
	tags       []*domain.FeedTag
	lastCursor *time.Time
}

func (s *stubFetchFeedTagsPort) FetchFeedTags(_ context.Context, _ string, cursor *time.Time, _ int) ([]*domain.FeedTag, error) {
	s.lastCursor = cursor
	return s.tags, nil
}

func TestRestHandleFetchFeedTagsByID_CursorRoundTripKeepsSubSecondPrecision(t *testing.T) {
	logger.InitLogger()

	ts := time.Date(2026, time.March, 2, 10, 0, 0, 123456000, time.UTC)
	stub := &stubFetchFeedTagsPort{
		tags: []*domain.FeedTag{
			{
				ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111").String(),
				TagName:   "golang",
				CreatedAt: ts,
			},
		},
	}

	container := &di.ApplicationComponents{
		FetchFeedTagsByIDUsecase: fetch_feed_tags_by_id_usecase.NewFetchFeedTagsByIDUsecase(stub),
	}
	handler := RestHandleFetchFeedTagsByID(container)
	e := echo.New()
	feedID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()

	req1 := httptest.NewRequest(http.MethodGet, "/v1/feeds/"+feedID+"/tags?limit=1", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("id")
	c1.SetParamValues(feedID)

	require.NoError(t, handler(c1))
	require.Equal(t, http.StatusOK, rec1.Code)

	var body1 struct {
		NextCursor string `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &body1))
	require.NotEmpty(t, body1.NextCursor)

	req2 := httptest.NewRequest(http.MethodGet, "/v1/feeds/"+feedID+"/tags?limit=1&cursor="+url.QueryEscape(body1.NextCursor), nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(feedID)

	require.NoError(t, handler(c2))
	require.Equal(t, http.StatusOK, rec2.Code)
	require.NotNil(t, stub.lastCursor)
	assert.True(t, stub.lastCursor.Equal(ts),
		"cursor %q became %q", ts.Format(time.RFC3339Nano), stub.lastCursor.Format(time.RFC3339Nano))

	wholeSec := time.Date(2026, time.March, 2, 10, 0, 0, 0, time.UTC)
	req3 := httptest.NewRequest(http.MethodGet, "/v1/feeds/"+feedID+"/tags?limit=1&cursor="+url.QueryEscape(wholeSec.Format(time.RFC3339)), nil)
	rec3 := httptest.NewRecorder()
	c3 := e.NewContext(req3, rec3)
	c3.SetParamNames("id")
	c3.SetParamValues(feedID)

	require.NoError(t, handler(c3))
	require.Equal(t, http.StatusOK, rec3.Code)
	require.NotNil(t, stub.lastCursor)
	assert.True(t, stub.lastCursor.Equal(wholeSec))
}
