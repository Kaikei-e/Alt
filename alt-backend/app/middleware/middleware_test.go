package middleware

import (
	"alt/utils/logger"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", nil)
			if tt.requestIDHeader != "" {
				req.Header.Set("X-Request-ID", tt.requestIDHeader)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			middleware := RequestIDMiddleware()
			var requestIDFromContext string

			handler := func(c *echo.Context) error {
				// Extract request ID from context
				if reqID := c.Request().Context().Value(logger.RequestIDKey); reqID != nil {
					requestIDFromContext = reqID.(string)
				}
				return c.String(http.StatusOK, "test")
			}

			err := middleware(handler)(c)
			require.NoError(t, err)

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
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	middleware := RequestIDMiddleware()
	var requestIDFromContext string

	handler := func(c *echo.Context) error {
		// Extract request ID from context using the logger function
		if reqID := c.Request().Context().Value(logger.RequestIDKey); reqID != nil {
			requestIDFromContext = reqID.(string)
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	err := middleware(handler)(c)
	require.NoError(t, err)
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
				req = httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, reqBodyReader)
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Add request ID to context
			ctx := context.WithValue(c.Request().Context(), logger.RequestIDKey, "test-request-123")
			c.SetRequest(c.Request().WithContext(ctx))

			middleware := LoggingMiddleware(testLogger)

			handler := func(c *echo.Context) error {
				return c.String(tt.responseStatus, "test response")
			}

			err := middleware(handler)(c)
			require.NoError(t, err)
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

func TestLoggingMiddleware_CapturesStatusSizeLatencyAndErrors(t *testing.T) {
	parseRecords := func(buf *bytes.Buffer) []map[string]any {
		var records []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			var m map[string]any
			dec := json.NewDecoder(strings.NewReader(line))
			dec.UseNumber()
			require.NoError(t, dec.Decode(&m))
			records = append(records, m)
		}
		return records
	}

	t.Run("200 response logs status, response_size, duration_ms, method, path, remote_addr, user_agent", func(t *testing.T) {
		var buf bytes.Buffer
		testLogger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		e := echo.New()
		e.Use(LoggingMiddleware(testLogger))
		e.GET("/v1/articles", func(c *echo.Context) error {
			return c.String(http.StatusOK, "hello world")
		})

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/articles", nil)
		req.RemoteAddr = "198.51.100.10:1234"
		req.Header.Set("User-Agent", "test-agent/1.0")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		records := parseRecords(&buf)
		require.Len(t, records, 2)

		started := records[0]
		assert.Equal(t, "request started", started["msg"])
		assert.Equal(t, "GET", started["method"])
		assert.Equal(t, "/v1/articles", started["path"])
		assert.Equal(t, "198.51.100.10", started["remote_addr"])
		assert.Equal(t, "test-agent/1.0", started["user_agent"])

		completed := records[1]
		assert.Equal(t, "INFO", completed["level"])
		assert.Equal(t, "request completed", completed["msg"])
		assert.Equal(t, "GET", completed["method"])
		assert.Equal(t, "/v1/articles", completed["path"])
		assert.Equal(t, json.Number("200"), completed["status"])
		assert.Equal(t, json.Number("11"), completed["response_size"])
		_, hasDuration := completed["duration_ms"]
		assert.True(t, hasDuration)
	})

	t.Run("404 response logs WARN with status 404", func(t *testing.T) {
		var buf bytes.Buffer
		testLogger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		e := echo.New()
		e.Use(LoggingMiddleware(testLogger))

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/missing", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		records := parseRecords(&buf)
		require.Len(t, records, 3)

		completed := records[1]
		assert.Equal(t, "WARN", completed["level"])
		assert.Equal(t, "request completed", completed["msg"])
		assert.Equal(t, json.Number("404"), completed["status"])

		errRec := records[2]
		assert.Equal(t, "ERROR", errRec["level"])
		assert.Equal(t, "request error", errRec["msg"])
	})

	t.Run("handler error mapped to 500 logs ERROR with status 500", func(t *testing.T) {
		var buf bytes.Buffer
		testLogger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		e := echo.New()
		e.Use(LoggingMiddleware(testLogger))
		e.GET("/v1/fail", func(c *echo.Context) error {
			return errors.New("boom")
		})

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/fail", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		records := parseRecords(&buf)
		require.Len(t, records, 3)

		completed := records[1]
		assert.Equal(t, "ERROR", completed["level"])
		assert.Equal(t, "request completed", completed["msg"])
		assert.Equal(t, json.Number("500"), completed["status"])

		errRec := records[2]
		assert.Equal(t, "ERROR", errRec["level"])
		assert.Equal(t, "request error", errRec["msg"])
		assert.Equal(t, "boom", errRec["error"])
	})
}
