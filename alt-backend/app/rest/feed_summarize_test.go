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

func TestHandleSummarizeFeed(t *testing.T) {
	// Initialize logger for testing
	logger.InitLogger()

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		mockResponse   string
		mockStatusCode int
		expectedStatus int
		checkResponse  func(*testing.T, map[string]interface{})
		wantErr        bool
	}{
		{
			name: "should successfully summarize feed",
			requestBody: map[string]interface{}{
				"feed_url": "http://example.com/article",
			},
			mockResponse: `{"success":true,"summary":"これはテスト記事の日本語要約です。","article_id":"test-123"}`,
			mockStatusCode: http.StatusOK,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.True(t, resp["success"].(bool))
				assert.NotEmpty(t, resp["summary"])
			},
		},
		{
			name: "should return error for empty feed_url",
			requestBody: map[string]interface{}{
				"feed_url": "",
			},
			expectedStatus: http.StatusBadRequest,
			wantErr:        true,
		},
		{
			name:           "should return error for missing feed_url",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock article server
			var mockArticleServer *httptest.Server
			if tc.mockResponse != "" {
				mockArticleServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Return mock article content
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("This is a test article content."))
				}))
				defer mockArticleServer.Close()
			}

			// Create mock pre-processor server
			var mockPreProcessorServer *httptest.Server
			if tc.mockResponse != "" {
				mockPreProcessorServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v1/summarize", r.URL.Path)
					assert.Equal(t, "POST", r.Method)
					w.WriteHeader(tc.mockStatusCode)
					w.Write([]byte(tc.mockResponse))
				}))
				defer mockPreProcessorServer.Close()
			}

			// Setup Echo instance
			e := echo.New()

			// Create config with mock server URL
			cfg := &config.Config{
				PreProcessor: config.PreProcessorConfig{
					Enabled: true,
					URL:     "",
				},
			}
			if mockPreProcessorServer != nil {
				cfg.PreProcessor.URL = mockPreProcessorServer.URL
			}

			// Create minimal container for testing
			container := &di.ApplicationComponents{}

			// Update request body to use mock article server URL if available
			requestBody := tc.requestBody
			if mockArticleServer != nil {
				requestBody["feed_url"] = mockArticleServer.URL
			}

			// Create request
			jsonBody, err := json.Marshal(requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/feeds/summarize", bytes.NewReader(jsonBody))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Execute handler
			handler := handleSummarizeFeed(container, cfg)
			err = handler(c)

			if tc.wantErr {
				// For error cases, Echo returns HTTPError
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedStatus, rec.Code)

				if tc.checkResponse != nil {
					var response map[string]interface{}
					err = json.Unmarshal(rec.Body.Bytes(), &response)
					require.NoError(t, err)
					tc.checkResponse(t, response)
				}
			}
		})
	}
}
