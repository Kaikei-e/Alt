package rest

import (
	"alt/di"
	"alt/domain"
	"alt/orchestrator/usecase/fetch_articles_usecase"
	"alt/shared/usecase/fetch_articles_by_tag_usecase"
	"alt/utils/logger"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

// The article cursor handlers ask their usecase for limit+1 rows so they can
// answer has_more without a COUNT. The usecases reject anything above 100, so
// the handler ceiling must leave room for that extra probe row — clamping the
// caller's limit to 100 made the documented maximum page size come back as an
// opaque 500 instead of a page.

// limitRecordingCursorPort records the limit the handler asked for, so the
// test can prove the probe row stays inside the usecase's ceiling.
type limitRecordingCursorPort struct {
	articles  []*domain.Article
	lastLimit int
}

func (s *limitRecordingCursorPort) FetchArticlesWithCursor(_ context.Context, _ *time.Time, limit int) ([]*domain.Article, error) {
	s.lastLimit = limit
	return s.articles, nil
}

func (s *limitRecordingCursorPort) FetchArticleIDsWithCursor(_ context.Context, _ *time.Time, limit int) ([]uuid.UUID, error) {
	s.lastLimit = limit
	return nil, nil
}

type limitRecordingByTagPort struct {
	articles  []*domain.TagTrailArticle
	lastLimit int
}

func (s *limitRecordingByTagPort) FetchArticlesByTag(_ context.Context, _ string, _ *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	s.lastLimit = limit
	return s.articles, nil
}

func (s *limitRecordingByTagPort) FetchArticlesByTagName(_ context.Context, _ string, _ *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	s.lastLimit = limit
	return s.articles, nil
}

func recordArticleLimitRequest(t *testing.T, handler echo.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(domain.SetUserContext(req.Context(), &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "test-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}))
	rec := httptest.NewRecorder()

	if err := handler(echo.New().NewContext(req, rec)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	return rec
}

func TestHandleFetchArticlesCursor_MaxLimitIsServedNotRejected(t *testing.T) {
	logger.InitLogger()

	port := &limitRecordingCursorPort{}
	handler := handleFetchArticlesCursor(&di.ApplicationComponents{
		FetchArticlesCursorUsecase: fetch_articles_usecase.NewFetchArticlesCursorUsecase(port),
	})

	rec := recordArticleLimitRequest(t, handler, "/v1/articles/fetch/cursor?limit=100")

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.LessOrEqual(t, port.lastLimit, 100,
		"the has_more probe row pushed the fetch past the usecase ceiling")
}

func TestHandleFetchArticlesByTag_MaxLimitIsServedNotRejected(t *testing.T) {
	logger.InitLogger()

	port := &limitRecordingByTagPort{}
	handler := handleFetchArticlesByTag(&di.ApplicationComponents{
		FetchArticlesByTagUsecase: fetch_articles_by_tag_usecase.NewFetchArticlesByTagUsecase(port),
	})

	rec := recordArticleLimitRequest(t, handler, "/v1/articles/by-tag?tag_name=go&limit=100")

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.LessOrEqual(t, port.lastLimit, 100,
		"the has_more probe row pushed the fetch past the usecase ceiling")
}

// An over-sized limit must be clamped down to the same served ceiling rather
// than rejected, which is what the handlers already promised by clamping.
func TestHandleFetchArticlesCursor_OverMaxLimitIsClampedNotRejected(t *testing.T) {
	logger.InitLogger()

	port := &limitRecordingCursorPort{}
	handler := handleFetchArticlesCursor(&di.ApplicationComponents{
		FetchArticlesCursorUsecase: fetch_articles_usecase.NewFetchArticlesCursorUsecase(port),
	})

	rec := recordArticleLimitRequest(t, handler, "/v1/articles/fetch/cursor?limit=50000")

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.LessOrEqual(t, port.lastLimit, 100,
		"the has_more probe row pushed the fetch past the usecase ceiling")
}
