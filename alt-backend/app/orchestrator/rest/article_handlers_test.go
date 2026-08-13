package rest

import (
	"alt/di"
	"alt/domain"
	"alt/mocks"
	"alt/orchestrator/gateway/fetch_article_gateway"
	"alt/orchestrator/gateway/robots_txt_gateway"
	"alt/orchestrator/usecase/fetch_article_usecase"
	"alt/orchestrator/usecase/fetch_articles_usecase"
	"alt/shared/usecase/fetch_articles_by_tag_usecase"
	// Note: driver/gateway imports are used to construct the usecase, not for direct handler access.
	"alt/utils/logger"
	"alt/utils/security"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// stubArticleRepository stands in for the repository fetch_article_usecase
// depends on.
//
// It used to be *alt_db.AltDBRepository over a pgxmock pool, and these three
// subtests were written as SQL expectations: an empty rows set for the article
// lookup, a boolean row for declined_domains, an INSERT for the save. That
// stopped being possible in ADR-000954 Wave 3 — cmd/backend has no pool, and
// this handler's repository is now three alt-data-hub gateways behind one
// interface — and it stopped being desirable at the same time. What these
// subtests are about is the *handler's* branching: declined means 403, robots
// disallow means "record the decline and 403", allowed means 200 with the
// extracted text. None of that is a claim about SQL, and asserting it through
// query regexes made the test fail whenever a column moved.
//
// The queries themselves are covered where they belong: the driver tests in
// shared/driver/alt_db, and the CDC pacts in
// shared/gateway/datahub_gateway/contract for the wire shape between them.
type stubArticleRepository struct {
	existing *domain.ArticleContent
	declined bool

	declinedSaved  []string
	savedArticleID string
	saveArticleErr error
}

func (s *stubArticleRepository) FetchArticleByURL(_ context.Context, _ string) (*domain.ArticleContent, error) {
	return s.existing, nil
}

func (s *stubArticleRepository) IsDomainDeclined(_ context.Context, _, _ string) (bool, error) {
	return s.declined, nil
}

func (s *stubArticleRepository) SaveDeclinedDomain(_ context.Context, _, domainStr string) error {
	s.declinedSaved = append(s.declinedSaved, domainStr)
	return nil
}

func (s *stubArticleRepository) SaveArticle(_ context.Context, _, _, _ string) (string, error) {
	if s.saveArticleErr != nil {
		return "", s.saveArticleErr
	}
	return s.savedArticleID, nil
}

func (s *stubArticleRepository) SaveArticleHead(_ context.Context, _, _, _ string) error { return nil }

func (s *stubArticleRepository) FetchArticleHeadByArticleID(_ context.Context, _ string) (*domain.ArticleHead, error) {
	return nil, nil
}

func (s *stubArticleRepository) FetchOgImageURLByArticleID(_ context.Context, _ string) (string, error) {
	return "", nil
}

// MockRoundTripper for intercepting HTTP requests
type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

func TestHandleFetchArticle_Compliance(t *testing.T) {
	// Initialize Logger
	logger.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. Setup
	mockTransport := &MockRoundTripper{}
	mockHttpClient := &http.Client{Transport: mockTransport}
	// We skip strict SSRF validation for tests or ensure we use allowed domains
	ssrfValidator := security.NewSSRFValidator()

	repo := &stubArticleRepository{savedArticleID: uuid.New().String()}
	gw := robots_txt_gateway.NewRobotsTxtGatewayWithDeps(mockHttpClient, ssrfValidator)
	// Inject Gateway with deps (injecting mockHttpClient allows intercepting fetch article request)
	fetchGw := fetch_article_gateway.NewFetchArticleGatewayWithDeps(nil, mockHttpClient, ssrfValidator)

	// Mock RAG Integration
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	// Create real Usecase composed of mocks/stubs
	// Note: NewArticleUsecase expects (FetchArticlePort, RobotsTxtPort, ArticleRepository, RagIntegrationPort)
	articleUsecase := fetch_article_usecase.NewArticleUsecase(fetchGw, gw, repo, mockRag)

	// Partial container with only needed components. There is no repository
	// field to populate any more: the handler reaches its data through the
	// usecase, and cmd/backend's container carries no database handle at all
	// since ADR-000954 Wave 3 batch 6.
	container := &di.ApplicationComponents{
		ArticleUsecase: articleUsecase,
	}

	userID := uuid.New()
	targetURLStr := "https://example.com/article"
	domainStr := "example.com"

	// Helper to create context with user
	createContext := func() (echo.Context, *httptest.ResponseRecorder) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/?url="+url.QueryEscape(targetURLStr), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Set User Context
		uCtx := &domain.UserContext{
			UserID:    userID,
			Email:     "test@example.com",
			ExpiresAt: time.Now().Add(time.Hour),
		}
		c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), domain.UserContextKey, uCtx)))
		return c, rec
	}

	// Safety fallback
	mockTransport.RoundTripFunc = func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected call to %s", req.URL.String())
	}

	t.Run("Already Declined", func(t *testing.T) {
		c, rec := createContext()

		// No stored article, and the domain has already declined us.
		repo.existing = nil
		repo.declined = true
		repo.declinedSaved = nil

		handler := handleFetchArticle(container)
		err := handler(c)

		// Assertions
		assert.NoError(t, err) // Handler returns error via c.JSON usually, or nil if handled
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "The request was declined")
		// A domain already on the list is not written again — the check short
		// circuits before robots.txt is ever fetched.
		assert.Empty(t, repo.declinedSaved)
	})

	t.Run("Robots.txt Disallowed", func(t *testing.T) {
		c, rec := createContext()

		repo.existing = nil
		repo.declined = false
		repo.declinedSaved = nil

		// Mock HTTP: robots.txt Disallow: /article
		mockTransport.RoundTripFunc = func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.String(), "robots.txt") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("User-agent: *\nDisallow: /article")),
					Header:     make(http.Header),
				}, nil
			}
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}

		handler := handleFetchArticle(container)
		err := handler(c)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "The request was declined")
		// The refusal is recorded, so the next reader does not re-fetch
		// robots.txt to be told the same thing.
		assert.Equal(t, []string{domainStr}, repo.declinedSaved)
	})

	t.Run("Allowed and Fetched", func(t *testing.T) {
		c, rec := createContext()

		repo.existing = nil
		repo.declined = false
		repo.declinedSaved = nil

		// Mock HTTP: robots.txt Allowed
		mockTransport.RoundTripFunc = func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.String(), "robots.txt") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("User-agent: *\nAllow: /")),
					Header:     make(http.Header),
				}, nil
			}
			// Mock Article Content Fetch
			if req.URL.String() == targetURLStr {
				// We need to return valid HTML for text extraction
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("<html><head><title>Title</title></head><body><h1>Title</h1><p>Content</p></body></html>")),
					Header:     make(http.Header),
				}, nil
			}
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}

		// The save itself — the upsert, the outbox insert and the transaction
		// holding them together — is one alt-data-hub procedure now
		// (ArchiveArticle, catalog §2.B). Its atomicity is asserted where it
		// lives, in the driver test and in the pact; here the article simply
		// comes back with an id.

		// RAG upsert is async (goroutine); use AnyTimes to avoid race with ctrl.Finish
		mockRag.EXPECT().UpsertArticle(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		handler := handleFetchArticle(container)
		err := handler(c)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Content") // Extracted text

		// Allow the async RAG goroutine to complete before the gomock
		// controller finishes.
		time.Sleep(50 * time.Millisecond)
		assert.Empty(t, repo.declinedSaved, "an allowed fetch must not record a decline")
	})
}

// stubArticleCursorPort records the cursor the handler passed down, so the
// page-2 request can be compared against the timestamp page 1 handed out.
type stubArticleCursorPort struct {
	articles   []*domain.Article
	lastCursor *time.Time
}

func (s *stubArticleCursorPort) FetchArticlesWithCursor(_ context.Context, cursor *time.Time, _ int) ([]*domain.Article, error) {
	s.lastCursor = cursor
	return s.articles, nil
}

func (s *stubArticleCursorPort) FetchArticleIDsWithCursor(_ context.Context, cursor *time.Time, _ int) ([]uuid.UUID, error) {
	s.lastCursor = cursor
	return nil, nil
}

type stubArticlesByTagPort struct {
	articles   []*domain.TagTrailArticle
	lastCursor *time.Time
}

func (s *stubArticlesByTagPort) FetchArticlesByTag(_ context.Context, _ string, cursor *time.Time, _ int) ([]*domain.TagTrailArticle, error) {
	s.lastCursor = cursor
	return s.articles, nil
}

func (s *stubArticlesByTagPort) FetchArticlesByTagName(_ context.Context, _ string, cursor *time.Time, _ int) ([]*domain.TagTrailArticle, error) {
	s.lastCursor = cursor
	return s.articles, nil
}

func callArticleHandler(t *testing.T, handler echo.HandlerFunc, target string, out interface{}) {
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
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// TestHandleFetchArticlesCursor_RoundTripKeepsSubSecondPrecision pins the
// pagination boundary: articles.created_at is microsecond precision and one
// harvester transaction stamps a whole batch inside the same second, so a
// cursor that only names the second is fed back into `created_at < $1` and
// drops the rest of that second from the timeline for good.
func TestHandleFetchArticlesCursor_RoundTripKeepsSubSecondPrecision(t *testing.T) {
	logger.InitLogger()

	firstPageTail := time.Date(2026, time.March, 2, 10, 0, 0, 123456000, time.UTC)
	sameSecond := firstPageTail.Add(-500 * time.Microsecond)

	port := &stubArticleCursorPort{articles: []*domain.Article{
		{ID: uuid.New(), Title: "first", URL: "https://example.com/1", PublishedAt: firstPageTail},
		{ID: uuid.New(), Title: "second", URL: "https://example.com/2", PublishedAt: sameSecond},
	}}
	handler := handleFetchArticlesCursor(&di.ApplicationComponents{
		FetchArticlesCursorUsecase: fetch_articles_usecase.NewFetchArticlesCursorUsecase(port),
	})

	var page1 ArticlesWithCursorResponse
	callArticleHandler(t, handler, "/v1/articles/fetch/cursor?limit=1", &page1)
	if !page1.HasMore || page1.NextCursor == nil {
		t.Fatalf("expected a next cursor, got %+v", page1)
	}

	var page2 ArticlesWithCursorResponse
	callArticleHandler(t, handler,
		"/v1/articles/fetch/cursor?limit=1&cursor="+url.QueryEscape(*page1.NextCursor), &page2)

	if port.lastCursor == nil || !port.lastCursor.Equal(firstPageTail) {
		t.Fatalf("cursor %s became %s, so every row between them is skipped",
			firstPageTail.Format(time.RFC3339Nano), port.lastCursor.Format(time.RFC3339Nano))
	}
}

// TestHandleFetchArticlesByTag_RoundTripKeepsSubSecondPrecision is the Tag
// Trail half of the same walk: its cursor is also articles.created_at compared
// with `<`.
func TestHandleFetchArticlesByTag_RoundTripKeepsSubSecondPrecision(t *testing.T) {
	logger.InitLogger()

	firstPageTail := time.Date(2026, time.March, 2, 10, 0, 0, 123456000, time.UTC)
	sameSecond := firstPageTail.Add(-500 * time.Microsecond)

	port := &stubArticlesByTagPort{articles: []*domain.TagTrailArticle{
		{ID: uuid.New().String(), Title: "first", Link: "https://example.com/1", PublishedAt: firstPageTail},
		{ID: uuid.New().String(), Title: "second", Link: "https://example.com/2", PublishedAt: sameSecond},
	}}
	handler := handleFetchArticlesByTag(&di.ApplicationComponents{
		FetchArticlesByTagUsecase: fetch_articles_by_tag_usecase.NewFetchArticlesByTagUsecase(port),
	})

	var page1 ArticlesByTagResponse
	callArticleHandler(t, handler, "/v1/articles/by-tag?tag_name=go&limit=1", &page1)
	if !page1.HasMore || page1.NextCursor == nil {
		t.Fatalf("expected a next cursor, got %+v", page1)
	}

	var page2 ArticlesByTagResponse
	callArticleHandler(t, handler,
		"/v1/articles/by-tag?tag_name=go&limit=1&cursor="+url.QueryEscape(*page1.NextCursor), &page2)

	if port.lastCursor == nil || !port.lastCursor.Equal(firstPageTail) {
		t.Fatalf("cursor %s became %s, so every row between them is skipped",
			firstPageTail.Format(time.RFC3339Nano), port.lastCursor.Format(time.RFC3339Nano))
	}
}
