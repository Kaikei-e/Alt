package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"pre-processor/handler"
	"pre-processor/models"
	"pre-processor/test/mocks"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func testLoggerSummarize() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only show errors in tests
	}))
}

// TestNewSummarizeHandler_Constructor tests the constructor
func TestNewSummarizeHandler_Constructor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
	mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
	mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
	// TODO: Generate mock for SummarizeJobRepository
	// mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	logger := testLoggerSummarize()

	// For now, pass nil - this test only checks constructor, not functionality
	h := handler.NewSummarizeHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, nil, logger)

	assert.NotNil(t, h)
}

// TestSummarizeHandler_HandleSummarize tests the summarization endpoint
func TestSummarizeHandler_HandleSummarize(t *testing.T) {
	tests := map[string]struct {
		setupMock    func(*mocks.MockExternalAPIRepository, *mocks.MockSummaryRepository, *mocks.MockArticleRepository)
		requestBody  map[string]interface{}
		expectedCode int
		validateResp func(t *testing.T, resp map[string]interface{})
		wantErr      bool
	}{
		"should successfully summarize article": {
			setupMock: func(m *mocks.MockExternalAPIRepository, s *mocks.MockSummaryRepository, a *mocks.MockArticleRepository) {
				a.EXPECT().
					FindByID(gomock.Any(), "test-123").
					Return(&models.Article{
						ID:      "test-123",
						UserID:  "user-456",
						Content: "This is a test article content",
						Title:   "Test Title",
					}, nil)
				m.EXPECT().
					SummarizeArticle(gomock.Any(), gomock.Any()).
					Return(&models.SummarizedContent{
						ArticleID:       "test-123",
						SummaryJapanese: "これはテスト記事の要約です。",
					}, nil)
				s.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			requestBody: map[string]interface{}{
				"content":    "This is a test article content",
				"article_id": "test-123",
			},
			expectedCode: http.StatusOK,
			validateResp: func(t *testing.T, resp map[string]interface{}) {
				assert.True(t, resp["success"].(bool))
				assert.Equal(t, "これはテスト記事の要約です。", resp["summary"])
				assert.Equal(t, "test-123", resp["article_id"])
			},
		},
		"should successfully fetch content from DB if empty": {
			setupMock: func(m *mocks.MockExternalAPIRepository, s *mocks.MockSummaryRepository, a *mocks.MockArticleRepository) {
				a.EXPECT().
					FindByID(gomock.Any(), "test-123").
					Return(&models.Article{
						ID:      "test-123",
						UserID:  "user-456",
						Content: "Fetched content from DB",
						Title:   "Fetched Title",
					}, nil)
				m.EXPECT().
					SummarizeArticle(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, article *models.Article) (*models.SummarizedContent, error) {
						assert.Equal(t, "Fetched content from DB", article.Content)
						return &models.SummarizedContent{
							ArticleID:       "test-123",
							SummaryJapanese: "DBからの要約",
						}, nil
					})
				s.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			requestBody: map[string]interface{}{
				"content":    "",
				"article_id": "test-123",
			},
			expectedCode: http.StatusOK,
			validateResp: func(t *testing.T, resp map[string]interface{}) {
				assert.True(t, resp["success"].(bool))
				assert.Equal(t, "DBからの要約", resp["summary"])
			},
		},
		"should return error for empty content and not found in DB": {
			setupMock: func(m *mocks.MockExternalAPIRepository, s *mocks.MockSummaryRepository, a *mocks.MockArticleRepository) {
				a.EXPECT().
					FindByID(gomock.Any(), "test-123").
					Return(nil, nil) // Not found
			},
			requestBody: map[string]interface{}{
				"content":    "",
				"article_id": "test-123",
			},
			expectedCode: http.StatusNotFound,
			wantErr:      true,
		},
		"should return error for missing article_id": {
			setupMock: func(m *mocks.MockExternalAPIRepository, s *mocks.MockSummaryRepository, a *mocks.MockArticleRepository) {
				// No expectation - should fail before calling mock
			},
			requestBody: map[string]interface{}{
				"content": "This is a test article content",
			},
			expectedCode: http.StatusBadRequest,
			wantErr:      true,
		},
		"should handle summarization API failure": {
			setupMock: func(m *mocks.MockExternalAPIRepository, s *mocks.MockSummaryRepository, a *mocks.MockArticleRepository) {
				a.EXPECT().
					FindByID(gomock.Any(), "test-123").
					Return(&models.Article{
						ID:      "test-123",
						UserID:  "user-456",
						Content: "This is a test article content",
						Title:   "Test Title",
					}, nil)
				m.EXPECT().
					SummarizeArticle(gomock.Any(), gomock.Any()).
					Return(nil, assert.AnError)
			},
			requestBody: map[string]interface{}{
				"content":    "This is a test article content",
				"article_id": "test-123",
			},
			expectedCode: http.StatusInternalServerError,
			wantErr:      true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
			mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			tc.setupMock(mockAPIRepo, mockSummaryRepo, mockArticleRepo)

			// TODO: Generate mock for SummarizeJobRepository
			h := handler.NewSummarizeHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, nil, testLoggerSummarize())

			// Create Echo instance and request
			e := echo.New()
			jsonBody, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", bytes.NewReader(jsonBody))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Execute handler
			err = h.HandleSummarize(c)

			if tc.wantErr {
				assert.Error(t, err)
				if httpErr, ok := err.(*echo.HTTPError); ok {
					assert.Equal(t, tc.expectedCode, httpErr.Code)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedCode, rec.Code)

				var response map[string]interface{}
				err = json.Unmarshal(rec.Body.Bytes(), &response)
				require.NoError(t, err)

				tc.validateResp(t, response)
			}
		})
	}
}

// TestSummarizeHandler_InvalidJSON tests invalid JSON handling
func TestSummarizeHandler_InvalidJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
	mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
	mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
	// TODO: Generate mock for SummarizeJobRepository
	h := handler.NewSummarizeHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, nil, testLoggerSummarize())

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", bytes.NewReader([]byte("invalid json")))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.HandleSummarize(c)
	assert.Error(t, err)
}
