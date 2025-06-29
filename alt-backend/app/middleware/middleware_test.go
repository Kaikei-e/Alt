package middleware

import (
	"alt/utils/logger"
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestRequestIDMiddleware(t *testing.T) {
	tests := []struct {
		name            string
		requestIDHeader string
		expectGenerated bool
	}{
		{
			name:            "generates request ID when none provided",
			requestIDHeader: "",
			expectGenerated: true,
		},
		{
			name:            "uses provided request ID",
			requestIDHeader: "existing-request-123",
			expectGenerated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.requestIDHeader != "" {
				req.Header.Set("X-Request-ID", tt.requestIDHeader)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			middleware := RequestIDMiddleware()
			var requestIDFromContext string

			handler := func(c echo.Context) error {
				// Extract request ID from context
				if reqID := c.Request().Context().Value(logger.RequestIDKey); reqID != nil {
					requestIDFromContext = reqID.(string)
				}
				return c.String(http.StatusOK, "test")
			}

			err := middleware(handler)(c)
			assert.NoError(t, err)

			// Check response header contains request ID
			responseRequestID := rec.Header().Get("X-Request-ID")
			assert.NotEmpty(t, responseRequestID)

			// Check context contains request ID
			assert.NotEmpty(t, requestIDFromContext)

			if tt.expectGenerated {
				// Should have generated a UUID
				assert.Len(t, responseRequestID, 36) // UUID length
				assert.NotEqual(t, tt.requestIDHeader, responseRequestID)
			} else {
				// Should use the provided request ID
				assert.Equal(t, tt.requestIDHeader, responseRequestID)
				assert.Equal(t, tt.requestIDHeader, requestIDFromContext)
			}

			// Request ID should be the same in response header and context
			assert.Equal(t, responseRequestID, requestIDFromContext)
		})
	}
}

func TestRequestIDMiddleware_Integration(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	middleware := RequestIDMiddleware()
	var requestIDFromContext string

	handler := func(c echo.Context) error {
		// Extract request ID from context using the logger function
		if reqID := c.Request().Context().Value(logger.RequestIDKey); reqID != nil {
			requestIDFromContext = reqID.(string)
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	err := middleware(handler)(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify request ID was set and propagated
	responseRequestID := rec.Header().Get("X-Request-ID")
	assert.NotEmpty(t, responseRequestID)
	assert.NotEmpty(t, requestIDFromContext)
	assert.Equal(t, responseRequestID, requestIDFromContext)

	// Verify it's a valid UUID format (36 characters with dashes)
	assert.Len(t, responseRequestID, 36)
	assert.Contains(t, responseRequestID, "-")
}

func TestLoggingMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		requestBody    string
		responseStatus int
		expectedLogs   []string
	}{
		{
			name:           "GET request logs properly",
			method:         http.MethodGet,
			path:           "/api/feeds",
			requestBody:    "",
			responseStatus: http.StatusOK,
			expectedLogs:   []string{"request started", "request completed", "method=GET", "path=/api/feeds", "status=200"},
		},
		{
			name:           "POST request with body logs properly",
			method:         http.MethodPost,
			path:           "/api/feeds/register",
			requestBody:    `{"url":"https://example.com/feed.xml"}`,
			responseStatus: http.StatusCreated,
			expectedLogs:   []string{"request started", "request completed", "method=POST", "path=/api/feeds/register", "status=201"},
		},
		{
			name:           "Error response logs properly",
			method:         http.MethodGet,
			path:           "/api/feeds/invalid",
			requestBody:    "",
			responseStatus: http.StatusNotFound,
			expectedLogs:   []string{"request started", "request completed", "method=GET", "path=/api/feeds/invalid", "status=404"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a logger that writes to a buffer for testing
			var buf bytes.Buffer
			testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

			e := echo.New()
			var req *http.Request
			if tt.requestBody != "" {
				reqBodyReader := strings.NewReader(tt.requestBody)
				req = httptest.NewRequest(tt.method, tt.path, reqBodyReader)
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Add request ID to context
			ctx := context.WithValue(c.Request().Context(), logger.RequestIDKey, "test-request-123")
			c.SetRequest(c.Request().WithContext(ctx))

			middleware := LoggingMiddleware(testLogger)

			handler := func(c echo.Context) error {
				return c.String(tt.responseStatus, "test response")
			}

			err := middleware(handler)(c)
			assert.NoError(t, err)
			assert.Equal(t, tt.responseStatus, rec.Code)

			// Check that expected log entries are present
			output := buf.String()
			for _, expectedLog := range tt.expectedLogs {
				assert.Contains(t, output, expectedLog, "Expected log entry %q not found in output: %s", expectedLog, output)
			}

			// Verify request ID is in logs
			assert.Contains(t, output, "request_id=test-request-123")
		})
	}
}
