package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testValidToken = "rag-secret-token-for-test-at-least-24-bytes"

func TestAPIAuthMiddleware_Echo(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	tests := []struct {
		name           string
		enabled        bool
		token          string
		path           string
		authHeader     string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "missing auth header returns 401",
			enabled:        true,
			token:          testValidToken,
			path:           "/v1/rag/retrieve",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"unauthorized"}`,
		},
		{
			name:           "wrong token returns 401",
			enabled:        true,
			token:          testValidToken,
			path:           "/v1/rag/retrieve",
			authHeader:     "Bearer wrong-token-value-here-12345",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"unauthorized"}`,
		},
		{
			name:           "malformed header without Bearer prefix returns 401",
			enabled:        true,
			token:          testValidToken,
			path:           "/v1/rag/retrieve",
			authHeader:     "Basic " + testValidToken,
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"unauthorized"}`,
		},
		{
			name:           "valid bearer token returns 200",
			enabled:        true,
			token:          testValidToken,
			path:           "/v1/rag/retrieve",
			authHeader:     "Bearer " + testValidToken,
			expectedStatus: http.StatusOK,
			expectedBody:   `{"status":"ok"}`,
		},
		{
			name:           "disabled auth permits request without token",
			enabled:        false,
			token:          "",
			path:           "/v1/rag/retrieve",
			authHeader:     "",
			expectedStatus: http.StatusOK,
			expectedBody:   `{"status":"ok"}`,
		},
		{
			name:           "healthz is exempt without token",
			enabled:        true,
			token:          testValidToken,
			path:           "/healthz",
			authHeader:     "",
			expectedStatus: http.StatusOK,
			expectedBody:   `{"status":"ok"}`,
		},
		{
			name:           "readyz is exempt without token",
			enabled:        true,
			token:          testValidToken,
			path:           "/readyz",
			authHeader:     "",
			expectedStatus: http.StatusOK,
			expectedBody:   `{"status":"ok"}`,
		},
		{
			name:           "metrics is exempt without token",
			enabled:        true,
			token:          testValidToken,
			path:           "/metrics",
			authHeader:     "",
			expectedStatus: http.StatusOK,
			expectedBody:   `{"status":"ok"}`,
		},
		{
			name:           "health prefix is exempt without token",
			enabled:        true,
			token:          testValidToken,
			path:           "/health/live",
			authHeader:     "",
			expectedStatus: http.StatusOK,
			expectedBody:   `{"status":"ok"}`,
		},
		{
			name:           "path starting with health but without slash is not exempt",
			enabled:        true,
			token:          testValidToken,
			path:           "/health_admin",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"unauthorized"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			mw := NewAPIAuthMiddleware(tc.token, tc.enabled, logger)
			e.Use(mw.EchoMiddleware())

			e.Any("/*", func(c echo.Context) error {
				return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
			})

			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			assert.Equal(t, tc.expectedStatus, rec.Code)
			if tc.expectedBody != "" {
				assert.JSONEq(t, tc.expectedBody, rec.Body.String())
			}
		})
	}
}

func TestAPIAuthMiddleware_WrapHandler(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mw := NewAPIAuthMiddleware(testValidToken, true, logger)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	})

	handler := mw.WrapHandler(backend)

	// Missing token
	req := httptest.NewRequest(http.MethodPost, "/services.augur.v2.AugurService/StreamChat", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.JSONEq(t, `{"error":"unauthorized"}`, rec.Body.String())

	// Wrong token
	req = httptest.NewRequest(http.MethodPost, "/services.augur.v2.AugurService/StreamChat", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Valid token
	req = httptest.NewRequest(http.MethodPost, "/services.augur.v2.AugurService/StreamChat", nil)
	req.Header.Set("Authorization", "Bearer "+testValidToken)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())

	// Exempt path
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "ok", rec.Body.String())
}
