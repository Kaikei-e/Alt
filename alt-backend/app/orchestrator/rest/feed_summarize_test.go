package rest

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/orchestrator/port/preprocessor_summarize_port"
	summarization "alt/orchestrator/rest/rest_feeds/summarization"
	"alt/orchestrator/usecase/summarize_article_usecase"
	"alt/utils/logger"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSummarizeFeed(t *testing.T) {
	// Initialize logger for testing
	logger.InitLogger()

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		wantErr        bool
		errorContains  string
	}{
		{
			name: "should return error for empty feed_url",
			requestBody: map[string]interface{}{
				"feed_url": "",
			},
			expectedStatus: http.StatusBadRequest,
			wantErr:        true,
			errorContains:  "feed_url is required",
		},
		{
			name:           "should return error for missing feed_url",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			wantErr:        true,
			errorContains:  "feed_url is required",
		},
		{
			name: "should return error for invalid URL format",
			requestBody: map[string]interface{}{
				"feed_url": "invalid-url",
			},
			expectedStatus: http.StatusInternalServerError,
			wantErr:        true,
			errorContains:  "Failed to check article existence",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup Echo instance
			e := echo.New()

			// Create config
			cfg := &config.Config{
				PreProcessor: config.PreProcessorConfig{
					Enabled: true,
					URL:     "",
				},
			}

			// Create container with a usecase wired against nil ports, which
			// is what makes the summarise attempt fail. The container has no
			// repository field to nil out any more — cmd/backend carries no
			// database handle since ADR-000954 Wave 3 batch 6.
			container := &di.ApplicationComponents{
				SummarizeArticleUsecase: summarize_article_usecase.NewUsecase(nil, nil, nil),
			}

			// Create request
			jsonBody, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)

			// Create authenticated context
			userCtx := &domain.UserContext{
				UserID:    uuid.New(),
				Email:     "test@example.com",
				Role:      domain.UserRoleUser,
				TenantID:  uuid.New(),
				SessionID: "test-session",
				LoginAt:   time.Now(),
				ExpiresAt: time.Now().Add(time.Hour),
			}
			ctx := domain.SetUserContext(context.Background(), userCtx)

			req := httptest.NewRequest(http.MethodPost, "/v1/feeds/summarize", bytes.NewReader(jsonBody))
			req = req.WithContext(ctx)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Execute handler
			handler := summarization.RestHandleSummarizeFeed(container, cfg)
			err = handler(c)

			if tc.wantErr {
				// For error cases, Echo returns HTTPError
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedStatus, rec.Code)
			}
		})
	}
}

type stubStreamArticleRepo struct {
	byID            *domain.ArticleContent
	savedID         string
	savedSummaryIDs []string
}

func (s *stubStreamArticleRepo) FetchArticleByURL(_ context.Context, _ string) (*domain.ArticleContent, error) {
	return nil, nil
}

func (s *stubStreamArticleRepo) FetchArticleByID(_ context.Context, _ string) (*domain.ArticleContent, error) {
	return s.byID, nil
}

func (s *stubStreamArticleRepo) SaveArticle(_ context.Context, _, _, _ string) (string, error) {
	return s.savedID, nil
}

func (s *stubStreamArticleRepo) SaveArticleSummary(_ context.Context, articleID, _, _, _ string) error {
	s.savedSummaryIDs = append(s.savedSummaryIDs, articleID)
	return nil
}

func (s *stubStreamArticleRepo) FetchArticleSummaryByArticleID(_ context.Context, _ string) (*domain.FeedSummary, error) {
	return nil, nil
}

type stubStreamPreProcessor struct{}

func (s *stubStreamPreProcessor) Summarize(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

func (s *stubStreamPreProcessor) StreamSummarize(_ context.Context, _, _, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("data: streamed summary\n\n")), nil
}

func (s *stubStreamPreProcessor) QueueSummarize(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (s *stubStreamPreProcessor) GetSummarizeStatus(_ context.Context, _ string) (*preprocessor_summarize_port.SummarizeStatus, error) {
	return nil, nil
}

func TestHandleSummarizeFeedStream_ArticleIDScope(t *testing.T) {
	logger.InitLogger()

	cfg := &config.Config{
		PreProcessor: config.PreProcessorConfig{
			Enabled: true,
			URL:     "http://preprocessor:9200",
		},
	}

	userCtx := &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "test-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	runStreamRequest := func(t *testing.T, repo *stubStreamArticleRepo, body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		e := echo.New()
		container := &di.ApplicationComponents{
			SummarizeArticleUsecase: summarize_article_usecase.NewUsecase(repo, &stubStreamPreProcessor{}, nil),
		}
		jsonBody, err := json.Marshal(body)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/feeds/summarize/stream", bytes.NewReader(jsonBody))
		req = req.WithContext(domain.SetUserContext(context.Background(), userCtx))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := summarization.RestHandleSummarizeFeedStream(container, cfg)
		require.NoError(t, handler(c))
		return rec
	}

	t.Run("unowned article_id with supplied content returns 404 matching nonexistent article_id", func(t *testing.T) {
		repoWithContent := &stubStreamArticleRepo{byID: nil}
		recWithContent := runStreamRequest(t, repoWithContent, map[string]any{
			"feed_url":   "https://example.com/article",
			"article_id": "11111111-1111-4111-8111-111111111111",
			"content":    "injected content for another user's article",
			"title":      "Injected Title",
		})
		assert.Equal(t, http.StatusNotFound, recWithContent.Code)
		assert.Empty(t, repoWithContent.savedSummaryIDs)

		repoWithoutContent := &stubStreamArticleRepo{byID: nil}
		recWithoutContent := runStreamRequest(t, repoWithoutContent, map[string]any{
			"feed_url":   "https://example.com/article",
			"article_id": "11111111-1111-4111-8111-111111111111",
		})
		assert.Equal(t, http.StatusNotFound, recWithoutContent.Code)
		assert.Empty(t, repoWithoutContent.savedSummaryIDs)
	})

	t.Run("content-only request without article_id keeps existing behavior", func(t *testing.T) {
		repo := &stubStreamArticleRepo{savedID: "22222222-2222-4222-8222-222222222222"}
		rec := runStreamRequest(t, repo, map[string]any{
			"feed_url": "https://example.com/new-article",
			"content":  "caller supplied content without article_id",
			"title":    "Caller Title",
		})
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, []string{"22222222-2222-4222-8222-222222222222"}, repo.savedSummaryIDs)
	})
}
