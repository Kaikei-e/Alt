package rest

import (
	"alt/config"
	"alt/di"
	"alt/utils/logger"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSPReportEndpoint(t *testing.T) {
	// Initialize logger for testing
	logger.InitLogger()

	// Setup Echo instance
	e := echo.New()

	// Create minimal container and config for testing
	container := &di.ApplicationComponents{}
	cfg := &config.Config{}

	// Register routes
	RegisterRoutes(e, container, cfg)

	tests := []struct {
		name           string
		method         string
		path           string
		body           map[string]interface{}
		expectedStatus int
		description    string
	}{
		{
			name:   "valid_csp_report",
			method: "POST",
			path:   "/security/csp-report",
			body: map[string]interface{}{
				"csp-report": map[string]interface{}{
					"document-uri":       "http://localhost:3000/",
					"referrer":           "",
					"blocked-uri":        "eval",
					"violated-directive": "script-src",
					"original-policy":    "default-src 'self'",
				},
			},
			expectedStatus: http.StatusNoContent,
			description:    "Valid CSP report should return 204 No Content",
		},
		{
			name:           "invalid_json",
			method:         "POST",
			path:           "/security/csp-report",
			body:           nil,
			expectedStatus: http.StatusNoContent,
			description:    "Invalid JSON should return 204 No Content to prevent retries",
		},
		{
			name:           "empty_body",
			method:         "POST",
			path:           "/security/csp-report",
			body:           map[string]interface{}{},
			expectedStatus: http.StatusNoContent,
			description:    "Empty body should return 204 No Content",
		},
		{
			name:           "get_method_not_allowed",
			method:         "GET",
			path:           "/security/csp-report",
			body:           nil,
			expectedStatus: http.StatusMethodNotAllowed,
			description:    "GET method should not be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			var rec *httptest.ResponseRecorder

			if tt.method == "POST" {
				var body []byte
				var err error
				if tt.body != nil {
					body, err = json.Marshal(tt.body)
					require.NoError(t, err)
				} else {
					body = []byte("invalid json")
				}

				req = httptest.NewRequest(tt.method, tt.path, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}

			rec = httptest.NewRecorder()

			// Serve the request
			e.ServeHTTP(rec, req)

			// Check status code
			assert.Equal(t, tt.expectedStatus, rec.Code, tt.description)

			// For successful CSP reports, body should be empty
			if tt.expectedStatus == http.StatusNoContent {
				assert.Empty(t, rec.Body.String(), "CSP report response should be empty")
			}
		})
	}
}
