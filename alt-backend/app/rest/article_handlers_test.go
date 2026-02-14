package rest

import (
	"alt/di"
	"alt/domain"
	"alt/driver/alt_db"
	"alt/gateway/fetch_article_gateway"
	"alt/gateway/robots_txt_gateway"
	"alt/mocks"
	"alt/usecase/fetch_article_usecase"
	// Note: driver/gateway imports are used to construct the usecase, not for direct handler access.
	"alt/utils/logger"
	"alt/utils/security"
	"context"
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
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

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
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer mockPool.Close()

	mockTransport := &MockRoundTripper{}
	mockHttpClient := &http.Client{Transport: mockTransport}
	// We skip strict SSRF validation for tests or ensure we use allowed domains
	ssrfValidator := security.NewSSRFValidator()

	repo := alt_db.NewAltDBRepository(mockPool)
	gw := robots_txt_gateway.NewRobotsTxtGatewayWithDeps(mockHttpClient, ssrfValidator)
	// Inject Gateway with deps (injecting mockHttpClient allows intercepting fetch article request)
	fetchGw := fetch_article_gateway.NewFetchArticleGatewayWithDeps(nil, mockHttpClient, ssrfValidator)

	// Mock RAG Integration
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	// Create real Usecase composed of mocks/stubs
	// Note: NewArticleUsecase expects (FetchArticlePort, RobotsTxtPort, ArticleRepository, RagIntegrationPort)
	// repo is *AltDBRepository which uses mockPool, so it satisfies ArticleRepository interface.
	articleUsecase := fetch_article_usecase.NewArticleUsecase(fetchGw, gw, repo, mockRag)

	// Partial container with only needed components
	container := &di.ApplicationComponents{
		AltDBRepository: repo,
		ArticleUsecase:  articleUsecase,
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

	t.Run("Already Declined in DB", func(t *testing.T) {
		c, rec := createContext()

		// Mock: FetchArticleByURL -> Not Found (nil)
		mockPool.ExpectQuery(`(?is)SELECT id, .* FROM articles WHERE url = \$1`).
			WithArgs(targetURLStr).
			WillReturnRows(pgxmock.NewRows([]string{"id", "url", "title", "content", "created_at", "updated_at"})) // Empty means not found

		// Mock: IsDomainDeclined -> True
		mockPool.ExpectQuery(`(?is)SELECT EXISTS.*FROM declined_domains`).
			WithArgs(userID.String(), domainStr).
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))

		handler := handleFetchArticle(container)
		err := handler(c)

		// Assertions
		assert.NoError(t, err) // Handler returns error via c.JSON usually, or nil if handled
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "The request was declined")

		if err := mockPool.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})

	t.Run("Robots.txt Disallowed", func(t *testing.T) {
		c, rec := createContext()

		// Mock: FetchArticleByURL -> Not Found
		mockPool.ExpectQuery(`(?is)SELECT id, .* FROM articles WHERE url = \$1`).
			WithArgs(targetURLStr).
			WillReturnRows(pgxmock.NewRows([]string{"id", "url", "title", "content", "created_at", "updated_at"}))

		// Mock: IsDomainDeclined -> False
		mockPool.ExpectQuery(`(?is)SELECT EXISTS.*FROM declined_domains`).
			WithArgs(userID.String(), domainStr).
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

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

		// Mock: SaveDeclinedDomain -> Success
		mockPool.ExpectExec("INSERT INTO declined_domains").
			WithArgs(userID.String(), domainStr, pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		handler := handleFetchArticle(container)
		err := handler(c)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "The request was declined")

		if err := mockPool.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})

	t.Run("Allowed and Fetched", func(t *testing.T) {
		c, rec := createContext()

		// Mock: FetchArticleByURL -> Not Found
		mockPool.ExpectQuery(`(?is)SELECT id, .* FROM articles WHERE url = \$1`).
			WithArgs(targetURLStr).
			WillReturnRows(pgxmock.NewRows([]string{"id", "url", "title", "content", "created_at", "updated_at"}))

		// Mock: IsDomainDeclined -> False
		mockPool.ExpectQuery(`(?is)SELECT EXISTS.*FROM declined_domains`).
			WithArgs(userID.String(), domainStr).
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

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

		// Mock: GetFeedIDByURL -> Not Found (used inside SaveArticle)
		// We expect this to fail or return empty, and SaveArticle handles it.
		// GetFeedIDByURL uses separate query: SELECT id FROM feeds WHERE link = $1
		mockPool.ExpectQuery(`(?is)SELECT id FROM feeds WHERE link = \$1`).
			WithArgs(targetURLStr).
			WillReturnRows(pgxmock.NewRows([]string{"id"})) // Returns empty, so Scan returns ErrNoRows

		// Expect Transaction
		mockPool.ExpectBegin()

		// Expect Upsert Article
		mockPool.ExpectQuery("(?is)INSERT INTO articles").
			WithArgs("Title", pgxmock.AnyArg(), targetURLStr, userID, nil).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(uuid.New()))

		// Expect Outbox Event Insert
		mockPool.ExpectExec("(?is)INSERT INTO outbox_events").
			WithArgs("ARTICLE_UPSERT", pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		// Expect Commit
		mockPool.ExpectCommit()

		// Expect UpsertArticle to be called (best effort, so ignore error return)
		mockRag.EXPECT().UpsertArticle(gomock.Any(), gomock.Any()).Return(nil).Times(1)

		handler := handleFetchArticle(container)
		err := handler(c)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Content") // Extracted text

		if err := mockPool.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})
}
