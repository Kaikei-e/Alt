package recap

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"

	recapv2 "alt/gen/proto/alt/recap/v2"

	"alt/domain"
	"alt/usecase/recap_articles_usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockRecapArticlesUsecase is a mock implementation of RecapArticlesUsecaseInterface
type MockRecapArticlesUsecase struct {
	mock.Mock
}

func (m *MockRecapArticlesUsecase) Execute(ctx context.Context, input recap_articles_usecase.Input) (*domain.RecapArticlesPage, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RecapArticlesPage), args.Error(1)
}

// newHandlerForTest builds a Handler with mock usecases, bypassing the
// production DI wiring. The recapUsecase parameter is reserved for summary
// tests (pass nil when only articles are exercised).
func newHandlerForTest(recapUsecase RecapUsecaseInterface, articlesUsecase RecapArticlesUsecaseInterface, logger *slog.Logger) *Handler {
	return &Handler{
		recapUsecaseInterface: recapUsecase,
		articlesUsecase:       articlesUsecase,
		logger:                logger,
	}
}

// MockRecapUsecase is a mock implementation of RecapUsecase
type MockRecapUsecase struct {
	mock.Mock
}

func (m *MockRecapUsecase) GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RecapSummary), args.Error(1)
}

func (m *MockRecapUsecase) GetThreeDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RecapSummary), args.Error(1)
}

func (m *MockRecapUsecase) GetEveningPulse(ctx context.Context, date string) (*domain.EveningPulse, error) {
	args := m.Called(ctx, date)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.EveningPulse), args.Error(1)
}

func (m *MockRecapUsecase) SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]*domain.RecapSearchResult, error) {
	args := m.Called(ctx, tagName, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.RecapSearchResult), args.Error(1)
}

func (m *MockRecapUsecase) SearchRecapsByQuery(ctx context.Context, query string, limit int) ([]*domain.RecapSearchResult, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.RecapSearchResult), args.Error(1)
}

func TestHandler_GetEveningPulse(t *testing.T) {
	logger := slog.Default()

	t.Run("success - authenticated user with 3 topics", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		tier1Count := 5
		trendMultiplier := 4.2
		genre := "Technology"
		expectedPulse := &domain.EveningPulse{
			JobID:       "job-123",
			Date:        "2026-01-31",
			GeneratedAt: time.Date(2026, 1, 31, 18, 0, 0, 0, time.UTC),
			Status:      domain.PulseStatusNormal,
			Topics: []domain.PulseTopic{
				{
					ClusterID:    12345,
					Role:         domain.TopicRoleNeedToKnow,
					Title:        "日銀利上げ決定",
					Rationale:    domain.PulseRationale{Text: "12媒体が報道", Confidence: domain.ConfidenceHigh},
					ArticleCount: 45,
					SourceCount:  12,
					Tier1Count:   &tier1Count,
					TimeAgo:      "3時間前",
					Genre:        &genre,
					ArticleIDs:   []string{"art-001", "art-002"},
				},
				{
					ClusterID:       12346,
					Role:            domain.TopicRoleTrend,
					Title:           "AI半導体急騰",
					Rationale:       domain.PulseRationale{Text: "4.2倍", Confidence: domain.ConfidenceHigh},
					ArticleCount:    28,
					SourceCount:     8,
					TimeAgo:         "1時間前",
					TrendMultiplier: &trendMultiplier,
					ArticleIDs:      []string{"art-010"},
				},
				{
					ClusterID:    12347,
					Role:         domain.TopicRoleSerendipity,
					Title:        "深海新種発見",
					Rationale:    domain.PulseRationale{Text: "Science", Confidence: domain.ConfidenceMedium},
					ArticleCount: 5,
					SourceCount:  3,
					TimeAgo:      "5時間前",
					ArticleIDs:   []string{"art-020"},
				},
			},
		}

		mockUsecase.On("GetEveningPulse", mock.Anything, "2026-01-31").Return(expectedPulse, nil)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		date := "2026-01-31"
		req := connect.NewRequest(&recapv2.GetEveningPulseRequest{Date: &date})
		resp, err := handler.GetEveningPulse(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "job-123", resp.Msg.JobId)
		assert.Equal(t, "2026-01-31", resp.Msg.Date)
		assert.Equal(t, recapv2.PulseStatus_PULSE_STATUS_NORMAL, resp.Msg.Status)
		assert.Len(t, resp.Msg.Topics, 3)

		// Verify first topic
		assert.Equal(t, int64(12345), resp.Msg.Topics[0].ClusterId)
		assert.Equal(t, recapv2.TopicRole_TOPIC_ROLE_NEED_TO_KNOW, resp.Msg.Topics[0].Role)
		assert.Equal(t, "日銀利上げ決定", resp.Msg.Topics[0].Title)
		require.NotNil(t, resp.Msg.Topics[0].Tier1Count)
		assert.Equal(t, int32(5), *resp.Msg.Topics[0].Tier1Count)

		// Verify second topic (Trend)
		assert.Equal(t, recapv2.TopicRole_TOPIC_ROLE_TREND, resp.Msg.Topics[1].Role)
		require.NotNil(t, resp.Msg.Topics[1].TrendMultiplier)
		assert.InDelta(t, 4.2, *resp.Msg.Topics[1].TrendMultiplier, 0.01)

		mockUsecase.AssertExpectations(t)
	})

	t.Run("success - quiet day", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		expectedPulse := &domain.EveningPulse{
			JobID:       "quiet-job",
			Date:        "2026-01-31",
			GeneratedAt: time.Now(),
			Status:      domain.PulseStatusQuietDay,
			Topics:      []domain.PulseTopic{},
			QuietDay: &domain.QuietDayInfo{
				Message: "今日は静かな一日でした。",
				WeeklyHighlights: []domain.WeeklyHighlight{
					{ID: "h1", Title: "Top News", Date: "2026-01-29", Role: "need_to_know"},
				},
			},
		}

		mockUsecase.On("GetEveningPulse", mock.Anything, "").Return(expectedPulse, nil)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		req := connect.NewRequest(&recapv2.GetEveningPulseRequest{})
		resp, err := handler.GetEveningPulse(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, recapv2.PulseStatus_PULSE_STATUS_QUIET_DAY, resp.Msg.Status)
		assert.Len(t, resp.Msg.Topics, 0)
		require.NotNil(t, resp.Msg.QuietDay)
		assert.Contains(t, resp.Msg.QuietDay.Message, "静かな一日")
		assert.Len(t, resp.Msg.QuietDay.WeeklyHighlights, 1)

		mockUsecase.AssertExpectations(t)
	})

	t.Run("unauthenticated - returns error", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)

		// No user context
		req := connect.NewRequest(&recapv2.GetEveningPulseRequest{})
		_, err := handler.GetEveningPulse(context.Background(), req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())

		mockUsecase.AssertNotCalled(t, "GetEveningPulse")
	})

	t.Run("not found - returns not found error", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		mockUsecase.On("GetEveningPulse", mock.Anything, "2026-01-31").
			Return(nil, domain.ErrEveningPulseNotFound)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		date := "2026-01-31"
		req := connect.NewRequest(&recapv2.GetEveningPulseRequest{Date: &date})
		_, err := handler.GetEveningPulse(ctx, req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeNotFound, connectErr.Code())

		mockUsecase.AssertExpectations(t)
	})

	t.Run("internal error - returns internal error", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		mockUsecase.On("GetEveningPulse", mock.Anything, "").
			Return(nil, errors.New("database error"))

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		req := connect.NewRequest(&recapv2.GetEveningPulseRequest{})
		_, err := handler.GetEveningPulse(ctx, req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeInternal, connectErr.Code())

		mockUsecase.AssertExpectations(t)
	})
}

func TestHandler_SearchRecapsByTag(t *testing.T) {
	logger := slog.Default()

	t.Run("free-text query takes precedence over tag_name", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		expectedResults := []*domain.RecapSearchResult{
			{
				JobID:      "job-001",
				ExecutedAt: "2026-04-01T00:00:00Z",
				WindowDays: 7,
				Genre:      "Technology",
				Summary:    "AI chip developments",
				TopTerms:   []string{"AI", "chips"},
				Bullets:    []string{"bullet1"},
			},
		}
		mockUsecase.On("SearchRecapsByQuery", mock.Anything, "artificial intelligence", 50).
			Return(expectedResults, nil)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		query := "artificial intelligence"
		req := connect.NewRequest(&recapv2.SearchRecapsByTagRequest{
			TagName: "AI",
			Query:   &query,
		})
		resp, err := handler.SearchRecapsByTag(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.Msg.Results, 1)
		assert.Equal(t, "job-001", resp.Msg.Results[0].JobId)
		assert.Equal(t, "AI chip developments", resp.Msg.Results[0].Summary)

		// SearchRecapsByTag should NOT be called when query is provided
		mockUsecase.AssertNotCalled(t, "SearchRecapsByTag")
		mockUsecase.AssertExpectations(t)
	})

	t.Run("query only - no tag_name", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		expectedResults := []*domain.RecapSearchResult{
			{
				JobID:      "job-002",
				ExecutedAt: "2026-04-02T00:00:00Z",
				WindowDays: 3,
				Genre:      "Finance",
				Summary:    "Market trends",
				TopTerms:   []string{"stocks", "bonds"},
				Bullets:    []string{"bullet2"},
			},
		}
		mockUsecase.On("SearchRecapsByQuery", mock.Anything, "market trends", 30).
			Return(expectedResults, nil)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		query := "market trends"
		req := connect.NewRequest(&recapv2.SearchRecapsByTagRequest{
			Limit: 30,
			Query: &query,
		})
		resp, err := handler.SearchRecapsByTag(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.Msg.Results, 1)
		assert.Equal(t, "job-002", resp.Msg.Results[0].JobId)

		mockUsecase.AssertNotCalled(t, "SearchRecapsByTag")
		mockUsecase.AssertExpectations(t)
	})

	t.Run("tag_name fallback when query is not provided", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		expectedResults := []*domain.RecapSearchResult{
			{
				JobID:      "job-003",
				ExecutedAt: "2026-04-03T00:00:00Z",
				WindowDays: 7,
				Genre:      "Science",
				Summary:    "Climate research",
				TopTerms:   []string{"climate"},
				Bullets:    []string{"bullet3"},
			},
		}
		mockUsecase.On("SearchRecapsByTag", mock.Anything, "climate", 50).
			Return(expectedResults, nil)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		req := connect.NewRequest(&recapv2.SearchRecapsByTagRequest{
			TagName: "climate",
		})
		resp, err := handler.SearchRecapsByTag(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.Msg.Results, 1)
		assert.Equal(t, "job-003", resp.Msg.Results[0].JobId)

		mockUsecase.AssertNotCalled(t, "SearchRecapsByQuery")
		mockUsecase.AssertExpectations(t)
	})

	t.Run("neither query nor tag_name - returns error", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		req := connect.NewRequest(&recapv2.SearchRecapsByTagRequest{})
		_, err := handler.SearchRecapsByTag(ctx, req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())

		mockUsecase.AssertNotCalled(t, "SearchRecapsByTag")
		mockUsecase.AssertNotCalled(t, "SearchRecapsByQuery")
	})

	t.Run("empty query string falls back to tag_name", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		expectedResults := []*domain.RecapSearchResult{
			{
				JobID:      "job-004",
				ExecutedAt: "2026-04-04T00:00:00Z",
				WindowDays: 3,
				Genre:      "Tech",
				Summary:    "Rust updates",
				TopTerms:   []string{"rust"},
				Bullets:    []string{"bullet4"},
			},
		}
		mockUsecase.On("SearchRecapsByTag", mock.Anything, "rust", 50).
			Return(expectedResults, nil)

		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New(), Email: "test@example.com", ExpiresAt: time.Now().Add(time.Hour)})

		emptyQuery := ""
		req := connect.NewRequest(&recapv2.SearchRecapsByTagRequest{
			TagName: "rust",
			Query:   &emptyQuery,
		})
		resp, err := handler.SearchRecapsByTag(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.Msg.Results, 1)

		mockUsecase.AssertNotCalled(t, "SearchRecapsByQuery")
		mockUsecase.AssertExpectations(t)
	})

	t.Run("unauthenticated - returns error", func(t *testing.T) {
		mockUsecase := new(MockRecapUsecase)
		handler := NewHandlerWithUsecase(mockUsecase, nil, logger)

		req := connect.NewRequest(&recapv2.SearchRecapsByTagRequest{TagName: "test"})
		_, err := handler.SearchRecapsByTag(context.Background(), req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
	})
}

func TestHandler_ListRecapArticles(t *testing.T) {
	logger := slog.Default()
	articleID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	title := "Test Article"
	sourceURL := "https://example.com/a"
	langHint := "en"
	publishedAt := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	buildPage := func() *domain.RecapArticlesPage {
		return &domain.RecapArticlesPage{
			Total:    1,
			Page:     1,
			PageSize: 500,
			HasMore:  false,
			Articles: []domain.RecapArticle{{
				ID:          articleID,
				Title:       &title,
				FullText:    "Body text here.",
				SourceURL:   &sourceURL,
				LangHint:    &langHint,
				PublishedAt: &publishedAt,
			}},
		}
	}

	t.Run("success - forwards input and maps domain to proto", func(t *testing.T) {
		mockArticles := new(MockRecapArticlesUsecase)
		mockArticles.On("Execute", mock.Anything, mock.MatchedBy(func(in recap_articles_usecase.Input) bool {
			return in.Page == 2 && in.PageSize == 100 &&
				in.From.Equal(time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)) &&
				in.To.Equal(time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
		})).Return(buildPage(), nil)

		handler := newHandlerForTest(nil, mockArticles, logger)
		pageReq := int32(2)
		pageSizeReq := int32(100)
		req := connect.NewRequest(&recapv2.ListRecapArticlesRequest{
			From:     "2026-04-14T00:00:00Z",
			To:       "2026-04-15T00:00:00Z",
			Page:     &pageReq,
			PageSize: &pageSizeReq,
		})

		resp, err := handler.ListRecapArticles(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(1), resp.Msg.Total)
		assert.Equal(t, int32(1), resp.Msg.Page)
		assert.Equal(t, int32(500), resp.Msg.PageSize)
		assert.False(t, resp.Msg.HasMore)
		require.Equal(t, "2026-04-14T00:00:00Z", resp.Msg.Range.From)
		require.Equal(t, "2026-04-15T00:00:00Z", resp.Msg.Range.To)
		require.Len(t, resp.Msg.Articles, 1)
		got := resp.Msg.Articles[0]
		assert.Equal(t, articleID.String(), got.ArticleId)
		require.NotNil(t, got.Title)
		assert.Equal(t, title, *got.Title)
		assert.Equal(t, "Body text here.", got.Fulltext)
		require.NotNil(t, got.PublishedAt)
		assert.Equal(t, "2026-04-15T12:00:00Z", *got.PublishedAt)
		mockArticles.AssertExpectations(t)
	})

	t.Run("invalid argument - from missing", func(t *testing.T) {
		mockArticles := new(MockRecapArticlesUsecase)
		handler := newHandlerForTest(nil, mockArticles, logger)

		req := connect.NewRequest(&recapv2.ListRecapArticlesRequest{
			To: "2026-04-15T00:00:00Z",
		})
		_, err := handler.ListRecapArticles(context.Background(), req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
		mockArticles.AssertNotCalled(t, "Execute", mock.Anything, mock.Anything)
	})

	t.Run("invalid argument - from not RFC3339", func(t *testing.T) {
		mockArticles := new(MockRecapArticlesUsecase)
		handler := newHandlerForTest(nil, mockArticles, logger)

		req := connect.NewRequest(&recapv2.ListRecapArticlesRequest{
			From: "yesterday",
			To:   "2026-04-15T00:00:00Z",
		})
		_, err := handler.ListRecapArticles(context.Background(), req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	})

	t.Run("usecase validation error maps to invalid argument", func(t *testing.T) {
		mockArticles := new(MockRecapArticlesUsecase)
		mockArticles.On("Execute", mock.Anything, mock.Anything).
			Return(nil, errors.New("page_size must be <= 2000"))

		handler := newHandlerForTest(nil, mockArticles, logger)
		req := connect.NewRequest(&recapv2.ListRecapArticlesRequest{
			From: "2026-04-14T00:00:00Z",
			To:   "2026-04-15T00:00:00Z",
		})
		_, err := handler.ListRecapArticles(context.Background(), req)

		require.Error(t, err)
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok)
		assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	})

	t.Run("empty page defaults to server side and maps empty articles to empty slice", func(t *testing.T) {
		mockArticles := new(MockRecapArticlesUsecase)
		mockArticles.On("Execute", mock.Anything, mock.MatchedBy(func(in recap_articles_usecase.Input) bool {
			return in.Page == 0 && in.PageSize == 0
		})).Return(&domain.RecapArticlesPage{
			Total:    0,
			Page:     1,
			PageSize: 500,
			HasMore:  false,
			Articles: nil,
		}, nil)

		handler := newHandlerForTest(nil, mockArticles, logger)
		req := connect.NewRequest(&recapv2.ListRecapArticlesRequest{
			From: "2026-04-14T00:00:00Z",
			To:   "2026-04-15T00:00:00Z",
		})

		resp, err := handler.ListRecapArticles(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, int32(0), resp.Msg.Total)
		assert.Empty(t, resp.Msg.Articles)
	})
}
