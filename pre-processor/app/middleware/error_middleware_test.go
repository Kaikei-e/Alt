// ABOUTME: Tests for centralized error handling middleware
// ABOUTME: Verifies error responses are secure and consistent
package middleware

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"

	apperrors "pre-processor/utils/errors"
)

func TestCustomHTTPErrorHandler_AppContextError(t *testing.T) {
	e := echo.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	e.HTTPErrorHandler = CustomHTTPErrorHandler(logger)

	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
		checkMessage   func(t *testing.T, msg string)
	}{
		{
			name:           "validation error shows message",
			err:            apperrors.NewValidationContextError("article ID is required", "handler", "Test", "Op", nil),
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "VALIDATION_ERROR",
			checkMessage: func(t *testing.T, msg string) {
				if msg != "article ID is required" {
					t.Errorf("expected message 'article ID is required', got %q", msg)
				}
			},
		},
		{
			name:           "not found error shows message",
			err:            apperrors.NewNotFoundContextError("article not found", "handler", "Test", "Op", nil),
			expectedStatus: http.StatusNotFound,
			expectedCode:   "NOT_FOUND_ERROR",
			checkMessage: func(t *testing.T, msg string) {
				if msg != "article not found" {
					t.Errorf("expected message 'article not found', got %q", msg)
				}
			},
		},
		{
			name:           "internal error hides details",
			err:            apperrors.NewInternalContextError("panic: nil pointer", "handler", "Test", "Op", errors.New("segfault"), nil),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "INTERNAL_ERROR",
			checkMessage: func(t *testing.T, msg string) {
				if msg == "panic: nil pointer" {
					t.Error("internal error message should not be exposed")
				}
				if msg == "" {
					t.Error("message should not be empty")
				}
			},
		},
		{
			name:           "database error hides details",
			err:            apperrors.NewDatabaseContextError("pq: connection refused", "repository", "Test", "Op", nil, nil),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "DATABASE_ERROR",
			checkMessage: func(t *testing.T, msg string) {
				if msg == "pq: connection refused" {
					t.Error("database error details should not be exposed")
				}
			},
		},
		{
			name:           "external API error hides details",
			err:            apperrors.NewExternalAPIContextError("news-creator: 500 Internal Server Error", "gateway", "Test", "Op", nil, nil),
			expectedStatus: http.StatusBadGateway,
			expectedCode:   "EXTERNAL_API_ERROR",
			checkMessage: func(t *testing.T, msg string) {
				if msg == "news-creator: 500 Internal Server Error" {
					t.Error("external API error details should not be exposed")
				}
			},
		},
		{
			name:           "timeout error",
			err:            apperrors.NewTimeoutContextError("context deadline exceeded", "gateway", "Test", "Op", nil, nil),
			expectedStatus: http.StatusGatewayTimeout,
			expectedCode:   "TIMEOUT_ERROR",
			checkMessage: func(t *testing.T, msg string) {
				if msg == "" {
					t.Error("message should not be empty")
				}
			},
		},
		{
			name:           "rate limit error",
			err:            apperrors.NewRateLimitContextError("too many requests", "handler", "Test", "Op", nil, nil),
			expectedStatus: http.StatusTooManyRequests,
			expectedCode:   "RATE_LIMIT_ERROR",
			checkMessage: func(t *testing.T, msg string) {
				if msg == "" {
					t.Error("message should not be empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			e.HTTPErrorHandler(tt.err, c)

			if rec.Code != tt.expectedStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.expectedStatus)
			}

			var resp apperrors.SecureHTTPResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if resp.Error.Code != tt.expectedCode {
				t.Errorf("code = %q, want %q", resp.Error.Code, tt.expectedCode)
			}

			tt.checkMessage(t, resp.Error.Message)
		})
	}
}

func TestCustomHTTPErrorHandler_EchoHTTPError(t *testing.T) {
	e := echo.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	e.HTTPErrorHandler = CustomHTTPErrorHandler(logger)

	tests := []struct {
		name           string
		err            *echo.HTTPError
		expectedStatus int
	}{
		{
			name:           "echo bad request",
			err:            echo.NewHTTPError(http.StatusBadRequest, "invalid input"),
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "echo not found",
			err:            echo.NewHTTPError(http.StatusNotFound, "resource not found"),
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "echo internal error",
			err:            echo.NewHTTPError(http.StatusInternalServerError, "something went wrong"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			e.HTTPErrorHandler(tt.err, c)

			if rec.Code != tt.expectedStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.expectedStatus)
			}

			// Verify response is valid JSON
			var resp map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			// Should have error field
			if _, ok := resp["error"]; !ok {
				t.Error("response should have 'error' field")
			}
		})
	}
}

func TestCustomHTTPErrorHandler_UnknownError(t *testing.T) {
	e := echo.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	e.HTTPErrorHandler = CustomHTTPErrorHandler(logger)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Regular error (not AppContextError or HTTPError)
	e.HTTPErrorHandler(errors.New("something unexpected"), c)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var resp apperrors.SecureHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Should not expose internal error message
	if resp.Error.Message == "something unexpected" {
		t.Error("internal error message should not be exposed")
	}
}

func TestCustomHTTPErrorHandler_ErrorIDPresent(t *testing.T) {
	e := echo.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	e.HTTPErrorHandler = CustomHTTPErrorHandler(logger)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := apperrors.NewInternalContextError("test", "handler", "Test", "Op", nil, nil)
	e.HTTPErrorHandler(err, c)

	var resp apperrors.SecureHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error.ErrorID == "" {
		t.Error("ErrorID should be present for tracking")
	}
}

func TestCustomHTTPErrorHandler_RetryableFlag(t *testing.T) {
	e := echo.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	e.HTTPErrorHandler = CustomHTTPErrorHandler(logger)

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "timeout error is retryable",
			err:       apperrors.NewTimeoutContextError("timeout", "handler", "Test", "Op", nil, nil),
			retryable: true,
		},
		{
			name:      "rate limit error is retryable",
			err:       apperrors.NewRateLimitContextError("rate limit", "handler", "Test", "Op", nil, nil),
			retryable: true,
		},
		{
			name:      "external API error is retryable",
			err:       apperrors.NewExternalAPIContextError("api error", "handler", "Test", "Op", nil, nil),
			retryable: true,
		},
		{
			name:      "validation error is not retryable",
			err:       apperrors.NewValidationContextError("invalid", "handler", "Test", "Op", nil),
			retryable: false,
		},
		{
			name:      "not found error is not retryable",
			err:       apperrors.NewNotFoundContextError("not found", "handler", "Test", "Op", nil),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			e.HTTPErrorHandler(tt.err, c)

			var resp apperrors.SecureHTTPResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if resp.Error.Retryable != tt.retryable {
				t.Errorf("Retryable = %v, want %v", resp.Error.Retryable, tt.retryable)
			}
		})
	}
}

func TestCustomHTTPErrorHandler_ResponseNotCommitted(t *testing.T) {
	e := echo.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	e.HTTPErrorHandler = CustomHTTPErrorHandler(logger)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Simulate already committed response
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Committed = true

	err := apperrors.NewInternalContextError("test", "handler", "Test", "Op", nil, nil)
	e.HTTPErrorHandler(err, c)

	// Status should remain as originally committed (200), not change to 500
	if rec.Code != http.StatusOK {
		t.Errorf("status should remain %d when response is committed, got %d", http.StatusOK, rec.Code)
	}
}
