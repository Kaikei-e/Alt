package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"pre-processor/domain"
	"pre-processor/handler"
	"pre-processor/test/mocks"
	apperrors "pre-processor/utils/errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	logger := testLoggerSummarize()

	// jobRepo is nil here deliberately: this test only checks the constructor
	// wires the handler together, not job-queue functionality (see
	// TestSummarizeHandler_HandleSummarizeQueue / _HandleSummarizeStatus for
	// jobRepo-dependent behavior with a real mock).
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
					Return(&domain.Article{
						ID:      "test-123",
						UserID:  "user-456",
						Content: "This is a test article content",
						Title:   "Test Title",
					}, nil)
				m.EXPECT().
					SummarizeArticle(gomock.Any(), gomock.Any(), "high").
					Return(&domain.SummarizedContent{
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
					Return(&domain.Article{
						ID:      "test-123",
						UserID:  "user-456",
						Content: "Fetched content from DB",
						Title:   "Fetched Title",
					}, nil)
				m.EXPECT().
					SummarizeArticle(gomock.Any(), gomock.Any(), "high").
					DoAndReturn(func(_ context.Context, article *domain.Article, _ string) (*domain.SummarizedContent, error) {
						assert.Equal(t, "Fetched content from DB", article.Content)
						return &domain.SummarizedContent{
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
					Return(&domain.Article{
						ID:      "test-123",
						UserID:  "user-456",
						Content: "This is a test article content",
						Title:   "Test Title",
					}, nil)
				m.EXPECT().
					SummarizeArticle(gomock.Any(), gomock.Any(), "high").
					Return(nil, assert.AnError)
			},
			requestBody: map[string]interface{}{
				"content":    "This is a test article content",
				"article_id": "test-123",
			},
			// mapDomainErrorToHTTP's default branch wraps any non-domain
			// error (e.g. a generic external API failure) as
			// EXTERNAL_API_ERROR, which AppContextError.HTTPStatusCode
			// maps to 502 Bad Gateway — this service is acting as a
			// gateway to the summarizer API, so an upstream failure is
			// reported as a bad gateway, not an internal server error.
			expectedCode: http.StatusBadGateway,
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

			// jobRepo is nil: HandleSummarize never touches the job queue.
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
				require.Error(t, err)
				// A bare `err.(*echo.HTTPError)` check is a no-op here: this
				// handler always returns *apperrors.AppContextError, never
				// *echo.HTTPError, so that assertion previously never ran.
				// httpStatusOf resolves either error type to its intended
				// HTTP status so the expectedCode is actually verified.
				assert.Equal(t, tc.expectedCode, httpStatusOf(t, err))
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
	// jobRepo is nil: request body binding fails before the job queue is touched.
	h := handler.NewSummarizeHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, nil, testLoggerSummarize())

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", bytes.NewReader([]byte("invalid json")))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.HandleSummarize(c)
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, httpStatusOf(t, err),
		"invalid JSON body should map to a 400, not a server error")
}

// TestSummarizeHandler_DuplicateRequestPrevention tests that duplicate requests are rejected
func TestSummarizeHandler_DuplicateRequestPrevention(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
	mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
	mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
	h := handler.NewSummarizeHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, nil, testLoggerSummarize())

	// Use a unique article ID for this test to avoid conflicts with other tests
	articleID := "duplicate-test-" + t.Name()

	// Set up mocks for the first request - it will block in SummarizeArticle
	// Note: The duplicate check happens BEFORE FindByID, so second request won't call any mocks
	mockArticleRepo.EXPECT().
		FindByID(gomock.Any(), articleID).
		Return(&domain.Article{
			ID:      articleID,
			UserID:  "user-456",
			Content: "Test content for duplicate test",
			Title:   "Test Title",
		}, nil).
		Times(1) // Only called once by first request

	// First request will be slow
	firstRequestStarted := make(chan struct{})
	firstRequestDone := make(chan struct{})

	mockAPIRepo.EXPECT().
		SummarizeArticle(gomock.Any(), gomock.Any(), "high").
		DoAndReturn(func(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
			close(firstRequestStarted)
			<-firstRequestDone // Block until we signal completion
			return &domain.SummarizedContent{
				ArticleID:       articleID,
				SummaryJapanese: "要約",
			}, nil
		}).
		Times(1)

	mockSummaryRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	e := echo.New()

	// Start first request in goroutine
	var firstErr error
	firstDone := make(chan struct{})
	go func() {
		jsonBody, _ := json.Marshal(map[string]interface{}{
			"content":    "Test content for duplicate test",
			"article_id": articleID,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", bytes.NewReader(jsonBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		firstErr = h.HandleSummarize(c)
		close(firstDone)
	}()

	// Wait for first request to start processing
	<-firstRequestStarted

	// Send second request while first is still processing
	jsonBody, _ := json.Marshal(map[string]interface{}{
		"content":    "Test content for duplicate test",
		"article_id": articleID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", bytes.NewReader(jsonBody))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Second request should get 409 Conflict
	secondErr := h.HandleSummarize(c)

	// Verify second request got conflict error
	require.Error(t, secondErr, "second request should have returned an error")
	assert.Contains(t, secondErr.Error(), "already being processed",
		"second request should indicate article is already being processed")

	// Signal first request to complete
	close(firstRequestDone)

	// Wait for first request to complete
	<-firstDone

	// Verify first request completed successfully
	assert.NoError(t, firstErr, "first request should complete successfully")
}

// httpStatusOf extracts the HTTP status an error maps to, mirroring how the
// production error middleware (utils/errors.AppContextError.HTTPStatusCode)
// converts a handler error into a response code. Handler unit tests call the
// handler method directly (bypassing echo's router/HTTPErrorHandler), so this
// helper is required to make an assertion on the intended status code for
// error-path test cases; a bare `err.(*echo.HTTPError)` type assertion always
// fails here because handler errors are *apperrors.AppContextError, not
// *echo.HTTPError.
func httpStatusOf(t *testing.T, err error) int {
	t.Helper()
	var appErr *apperrors.AppContextError
	if errors.As(err, &appErr) {
		return appErr.HTTPStatusCode()
	}
	var echoErr *echo.HTTPError
	if errors.As(err, &echoErr) {
		return echoErr.Code
	}
	t.Fatalf("error %v (%T) is neither *apperrors.AppContextError nor *echo.HTTPError", err, err)
	return 0
}

// TestSummarizeHandler_HandleSummarizeQueue tests the async job-queue endpoint,
// including every branch of the idempotency guard (service.ShouldQueueSummarizeJob)
// and both repository error paths.
func TestSummarizeHandler_HandleSummarizeQueue(t *testing.T) {
	tests := map[string]struct {
		setupMock    func(*mocks.MockSummaryRepository, *mocks.MockSummarizeJobRepository)
		requestBody  map[string]interface{}
		expectedCode int
		validateResp func(t *testing.T, resp map[string]interface{})
		wantErr      bool
	}{
		"should queue job successfully": {
			setupMock: func(s *mocks.MockSummaryRepository, j *mocks.MockSummarizeJobRepository) {
				s.EXPECT().Exists(gomock.Any(), "article-1").Return(false, nil)
				j.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-1", gomock.Any()).Return(false, nil)
				j.EXPECT().HasInFlightJob(gomock.Any(), "article-1", gomock.Any()).Return(false, nil)
				j.EXPECT().HasDeadLetterJob(gomock.Any(), "article-1").Return(false, nil)
				j.EXPECT().HasRecentFailedJob(gomock.Any(), "article-1", gomock.Any()).Return(false, nil)
				j.EXPECT().CreateJob(gomock.Any(), "article-1").Return("job-123", nil)
			},
			requestBody:  map[string]interface{}{"article_id": "article-1"},
			expectedCode: http.StatusAccepted,
			validateResp: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "job-123", resp["job_id"])
				assert.Equal(t, "pending", resp["status"])
			},
		},
		"should skip when a summary already exists": {
			setupMock: func(s *mocks.MockSummaryRepository, j *mocks.MockSummarizeJobRepository) {
				s.EXPECT().Exists(gomock.Any(), "article-1").Return(true, nil)
			},
			requestBody:  map[string]interface{}{"article_id": "article-1"},
			expectedCode: http.StatusAccepted,
			validateResp: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "", resp["job_id"])
				assert.Equal(t, "skipped", resp["status"])
				assert.Contains(t, resp["message"], "summary_exists")
			},
		},
		"should skip when a recent successful job exists": {
			setupMock: func(s *mocks.MockSummaryRepository, j *mocks.MockSummarizeJobRepository) {
				s.EXPECT().Exists(gomock.Any(), "article-1").Return(false, nil)
				j.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-1", gomock.Any()).Return(true, nil)
			},
			requestBody:  map[string]interface{}{"article_id": "article-1"},
			expectedCode: http.StatusAccepted,
			validateResp: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "skipped", resp["status"])
				assert.Contains(t, resp["message"], "recent_success")
			},
		},
		"should skip when an in-flight job exists": {
			setupMock: func(s *mocks.MockSummaryRepository, j *mocks.MockSummarizeJobRepository) {
				s.EXPECT().Exists(gomock.Any(), "article-1").Return(false, nil)
				j.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-1", gomock.Any()).Return(false, nil)
				j.EXPECT().HasInFlightJob(gomock.Any(), "article-1", gomock.Any()).Return(true, nil)
			},
			requestBody:  map[string]interface{}{"article_id": "article-1"},
			expectedCode: http.StatusAccepted,
			validateResp: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "skipped", resp["status"])
				assert.Contains(t, resp["message"], "in_flight")
			},
		},
		"should return 500 when the idempotency guard fails": {
			setupMock: func(s *mocks.MockSummaryRepository, j *mocks.MockSummarizeJobRepository) {
				s.EXPECT().Exists(gomock.Any(), "article-1").Return(false, assert.AnError)
			},
			requestBody:  map[string]interface{}{"article_id": "article-1"},
			expectedCode: http.StatusInternalServerError,
			wantErr:      true,
		},
		"should return 500 when CreateJob fails": {
			setupMock: func(s *mocks.MockSummaryRepository, j *mocks.MockSummarizeJobRepository) {
				s.EXPECT().Exists(gomock.Any(), "article-1").Return(false, nil)
				j.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-1", gomock.Any()).Return(false, nil)
				j.EXPECT().HasInFlightJob(gomock.Any(), "article-1", gomock.Any()).Return(false, nil)
				j.EXPECT().HasDeadLetterJob(gomock.Any(), "article-1").Return(false, nil)
				j.EXPECT().HasRecentFailedJob(gomock.Any(), "article-1", gomock.Any()).Return(false, nil)
				j.EXPECT().CreateJob(gomock.Any(), "article-1").Return("", assert.AnError)
			},
			requestBody:  map[string]interface{}{"article_id": "article-1"},
			expectedCode: http.StatusInternalServerError,
			wantErr:      true,
		},
		"should return 400 for missing article_id": {
			setupMock:    func(s *mocks.MockSummaryRepository, j *mocks.MockSummarizeJobRepository) {},
			requestBody:  map[string]interface{}{},
			expectedCode: http.StatusBadRequest,
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
			mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
			tc.setupMock(mockSummaryRepo, mockJobRepo)

			h := handler.NewSummarizeHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, mockJobRepo, testLoggerSummarize())

			e := echo.New()
			jsonBody, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize/queue", bytes.NewReader(jsonBody))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err = h.HandleSummarizeQueue(c)

			if tc.wantErr {
				require.Error(t, err)
				assert.Equal(t, tc.expectedCode, httpStatusOf(t, err))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedCode, rec.Code)

			var response map[string]interface{}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
			tc.validateResp(t, response)
		})
	}
}

// TestSummarizeHandler_HandleSummarizeStatus tests the job-status polling
// endpoint, including the not-found vs. transient-DB-error distinction that
// callers rely on to decide whether to keep polling (see the comment on
// HandleSummarizeStatus about pgx.ErrNoRows).
func TestSummarizeHandler_HandleSummarizeStatus(t *testing.T) {
	fixedJobID := uuid.New()
	summary := "the summary text"
	errMsg := "llm call failed"

	tests := map[string]struct {
		jobIDParam   string
		setupMock    func(*mocks.MockSummarizeJobRepository)
		expectedCode int
		validateResp func(t *testing.T, resp map[string]interface{})
		wantErr      bool
	}{
		"should return completed job with summary": {
			jobIDParam: fixedJobID.String(),
			setupMock: func(j *mocks.MockSummarizeJobRepository) {
				j.EXPECT().GetJob(gomock.Any(), fixedJobID.String()).Return(&domain.SummarizeJob{
					JobID:     fixedJobID,
					ArticleID: "article-1",
					Status:    domain.SummarizeJobStatusCompleted,
					Summary:   &summary,
				}, nil)
			},
			expectedCode: http.StatusOK,
			validateResp: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "completed", resp["status"])
				assert.Equal(t, summary, resp["summary"])
				assert.Equal(t, "article-1", resp["article_id"])
				assert.Empty(t, resp["error_message"])
			},
		},
		"should return failed job with error message": {
			jobIDParam: fixedJobID.String(),
			setupMock: func(j *mocks.MockSummarizeJobRepository) {
				j.EXPECT().GetJob(gomock.Any(), fixedJobID.String()).Return(&domain.SummarizeJob{
					JobID:        fixedJobID,
					ArticleID:    "article-1",
					Status:       domain.SummarizeJobStatusFailed,
					ErrorMessage: &errMsg,
				}, nil)
			},
			expectedCode: http.StatusOK,
			validateResp: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "failed", resp["status"])
				assert.Equal(t, errMsg, resp["error_message"])
				assert.Empty(t, resp["summary"])
			},
		},
		"should return pending job without summary or error": {
			jobIDParam: fixedJobID.String(),
			setupMock: func(j *mocks.MockSummarizeJobRepository) {
				j.EXPECT().GetJob(gomock.Any(), fixedJobID.String()).Return(&domain.SummarizeJob{
					JobID:     fixedJobID,
					ArticleID: "article-1",
					Status:    domain.SummarizeJobStatusPending,
				}, nil)
			},
			expectedCode: http.StatusOK,
			validateResp: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "pending", resp["status"])
				assert.Empty(t, resp["summary"])
				assert.Empty(t, resp["error_message"])
			},
		},
		"should return 404 when job genuinely does not exist": {
			jobIDParam: "missing-job",
			setupMock: func(j *mocks.MockSummarizeJobRepository) {
				j.EXPECT().GetJob(gomock.Any(), "missing-job").Return(nil, pgx.ErrNoRows)
			},
			expectedCode: http.StatusNotFound,
			wantErr:      true,
		},
		"should return 500 (not 404) on a transient DB error": {
			jobIDParam: "some-job",
			setupMock: func(j *mocks.MockSummarizeJobRepository) {
				j.EXPECT().GetJob(gomock.Any(), "some-job").Return(nil, errors.New("connection reset by peer"))
			},
			expectedCode: http.StatusInternalServerError,
			wantErr:      true,
		},
		"should return 400 for missing job_id path param": {
			jobIDParam:   "",
			setupMock:    func(j *mocks.MockSummarizeJobRepository) {},
			expectedCode: http.StatusBadRequest,
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
			mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
			tc.setupMock(mockJobRepo)

			h := handler.NewSummarizeHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, mockJobRepo, testLoggerSummarize())

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/summarize/status/"+tc.jobIDParam, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("job_id")
			c.SetParamValues(tc.jobIDParam)

			err := h.HandleSummarizeStatus(c)

			if tc.wantErr {
				require.Error(t, err)
				assert.Equal(t, tc.expectedCode, httpStatusOf(t, err))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedCode, rec.Code)

			var response map[string]interface{}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
			tc.validateResp(t, response)
		})
	}
}
