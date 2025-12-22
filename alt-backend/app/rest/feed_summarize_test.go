package rest

import (
	"alt/config"
	"alt/di"
	summarization "alt/rest/rest_feeds/summarization"
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

			// Create container with nil repository (will cause database error)
			container := &di.ApplicationComponents{
				AltDBRepository: nil,
			}

			// Create request
			jsonBody, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/feeds/summarize", bytes.NewReader(jsonBody))
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
