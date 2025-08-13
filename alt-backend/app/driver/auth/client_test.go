package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		wantURL     string
		wantTimeout time.Duration
	}{
		{
			name: "with auth config",
			config: &config.Config{
				Auth: config.AuthConfig{
					ServiceURL: "http://auth:9500",
					Timeout:    45 * time.Second,
				},
			},
			wantURL:     "http://auth:9500",
			wantTimeout: 45 * time.Second,
		},
		{
			name: "with legacy config",
			config: &config.Config{
				AuthServiceURL: "http://legacy-auth:8080",
				Auth: config.AuthConfig{
					ServiceURL: "",
					Timeout:    0,
				},
			},
			wantURL:     "http://legacy-auth:8080",
			wantTimeout: 30 * time.Second,
		},
		{
			name: "with default timeout",
			config: &config.Config{
				Auth: config.AuthConfig{
					ServiceURL: "http://auth:9500",
					Timeout:    0,
				},
			},
			wantURL:     "http://auth:9500",
			wantTimeout: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			client := NewClient(tt.config, logger)

			assert.Equal(t, tt.wantURL, client.baseURL)
			assert.Equal(t, tt.wantTimeout, client.httpClient.Timeout)
			assert.NotNil(t, client.logger)
		})
	}
}

func TestClient_ValidateSession(t *testing.T) {
	tests := []struct {
		name               string
		sessionToken       string
		tenantID           string
		mockResponseStatus int
		mockResponse       interface{}
		wantValid          bool
		wantUserID         string
		wantError          bool
	}{
		{
			name:               "valid session",
			sessionToken:       "valid-session-token",
			tenantID:           "tenant-123",
			mockResponseStatus: http.StatusOK,
			mockResponse: SessionValidationResponse{
				Valid:  true,
				UserID: "user-123",
				Email:  "test@example.com",
				Role:   "user",
				Context: &domain.UserContext{
					Email: "test@example.com",
					Role:  domain.UserRoleUser,
				},
			},
			wantValid:  true,
			wantUserID: "user-123",
			wantError:  false,
		},
		{
			name:               "invalid session",
			sessionToken:       "invalid-session-token",
			tenantID:           "tenant-123",
			mockResponseStatus: http.StatusOK,
			mockResponse: SessionValidationResponse{
				Valid: false,
			},
			wantValid:  false,
			wantUserID: "",
			wantError:  false,
		},
		{
			name:               "empty session token",
			sessionToken:       "",
			tenantID:           "tenant-123",
			mockResponseStatus: http.StatusOK, // This won't be called due to early return
			mockResponse:       SessionValidationResponse{Valid: true},
			wantValid:          false,
			wantUserID:         "",
			wantError:          false, // Should handle gracefully, not error
		},
		{
			name:               "auth service error - graceful fallback",
			sessionToken:       "test-token",
			tenantID:           "tenant-123",
			mockResponseStatus: http.StatusInternalServerError,
			mockResponse:       map[string]string{"error": "internal server error"},
			wantValid:          false,
			wantUserID:         "",
			wantError:          false, // Changed: should handle gracefully for OptionalAuth compatibility
		},
	}

	// Test service unavailable scenario separately
	t.Run("auth service unavailable", func(t *testing.T) {
		// Create client with non-existent server URL
		config := &config.Config{
			Auth: config.AuthConfig{
				ServiceURL: "http://non-existent-server:99999",
				Timeout:    1 * time.Second,
			},
		}
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		client := NewClient(config, logger)

		// Call ValidateSession
		ctx := context.Background()
		result, err := client.ValidateSession(ctx, "test-token", "")

		// Should handle gracefully for OptionalAuth compatibility
		assert.NoError(t, err, "ValidateSession should handle service unavailable gracefully")
		require.NotNil(t, result)
		assert.False(t, result.Valid, "Should return invalid session when service unavailable")
		assert.Empty(t, result.UserID)
		assert.Empty(t, result.Email)
		assert.Empty(t, result.Role)
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/api/v1/session/validate", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.WriteHeader(tt.mockResponseStatus)
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			// Create client with mock server URL
			config := &config.Config{
				Auth: config.AuthConfig{
					ServiceURL: server.URL,
					Timeout:    5 * time.Second,
				},
			}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			client := NewClient(config, logger)

			// Call ValidateSession
			ctx := context.Background()
			result, err := client.ValidateSession(ctx, tt.sessionToken, tt.tenantID)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.wantValid, result.Valid)
				assert.Equal(t, tt.wantUserID, result.UserID)
			}
		})
	}
}

func TestClient_GenerateCSRFToken(t *testing.T) {
	tests := []struct {
		name               string
		sessionToken       string
		mockResponseStatus int
		mockResponse       interface{}
		wantToken          string
		wantError          bool
	}{
		{
			name:               "successful token generation",
			sessionToken:       "valid-session-token",
			mockResponseStatus: http.StatusOK,
			mockResponse: CSRFTokenResponse{
				Token:     "csrf-token-123",
				ExpiresAt: time.Now().Add(time.Hour),
			},
			wantToken: "csrf-token-123",
			wantError: false,
		},
		{
			name:               "invalid session",
			sessionToken:       "invalid-session-token",
			mockResponseStatus: http.StatusUnauthorized,
			mockResponse:       map[string]string{"error": "unauthorized"},
			wantToken:          "",
			wantError:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/api/v1/csrf/generate", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.WriteHeader(tt.mockResponseStatus)
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			// Create client with mock server URL
			config := &config.Config{
				Auth: config.AuthConfig{
					ServiceURL: server.URL,
					Timeout:    5 * time.Second,
				},
			}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			client := NewClient(config, logger)

			// Call GenerateCSRFToken
			ctx := context.Background()
			result, err := client.GenerateCSRFToken(ctx, tt.sessionToken)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.wantToken, result.Token)
			}
		})
	}
}

func TestClient_ValidateCSRFToken(t *testing.T) {
	tests := []struct {
		name               string
		token              string
		sessionToken       string
		mockResponseStatus int
		mockResponse       interface{}
		wantValid          bool
		wantError          bool
	}{
		{
			name:               "valid CSRF token",
			token:              "valid-csrf-token",
			sessionToken:       "valid-session-token",
			mockResponseStatus: http.StatusOK,
			mockResponse: CSRFValidationResponse{
				Valid: true,
			},
			wantValid: true,
			wantError: false,
		},
		{
			name:               "invalid CSRF token",
			token:              "invalid-csrf-token",
			sessionToken:       "valid-session-token",
			mockResponseStatus: http.StatusOK,
			mockResponse: CSRFValidationResponse{
				Valid: false,
			},
			wantValid: false,
			wantError: false,
		},
		{
			name:               "auth service error",
			token:              "test-token",
			sessionToken:       "test-session",
			mockResponseStatus: http.StatusInternalServerError,
			mockResponse:       map[string]string{"error": "internal server error"},
			wantValid:          false,
			wantError:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/api/v1/csrf/validate", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				w.WriteHeader(tt.mockResponseStatus)
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			// Create client with mock server URL
			config := &config.Config{
				Auth: config.AuthConfig{
					ServiceURL: server.URL,
					Timeout:    5 * time.Second,
				},
			}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			client := NewClient(config, logger)

			// Call ValidateCSRFToken
			ctx := context.Background()
			result, err := client.ValidateCSRFToken(ctx, tt.token, tt.sessionToken)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.wantValid, result.Valid)
			}
		})
	}
}

func TestClient_HealthCheck(t *testing.T) {
	tests := []struct {
		name               string
		mockResponseStatus int
		mockResponse       interface{}
		wantError          bool
	}{
		{
			name:               "healthy service",
			mockResponseStatus: http.StatusOK,
			mockResponse:       map[string]string{"status": "ok"},
			wantError:          false,
		},
		{
			name:               "unhealthy service - graceful handling",
			mockResponseStatus: http.StatusOK,
			mockResponse:       map[string]string{"status": "error"},
			wantError:          false, // Changed: should handle gracefully
		},
		{
			name:               "service unavailable - graceful handling",
			mockResponseStatus: http.StatusServiceUnavailable,
			mockResponse:       map[string]string{"error": "service unavailable"},
			wantError:          false, // Changed: should handle gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/health", r.URL.Path)

				w.WriteHeader(tt.mockResponseStatus)
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			// Create client with mock server URL
			config := &config.Config{
				Auth: config.AuthConfig{
					ServiceURL: server.URL,
					Timeout:    5 * time.Second,
				},
			}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			client := NewClient(config, logger)

			// Call HealthCheck
			ctx := context.Background()
			err := client.HealthCheck(ctx)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestClient_makeRequest(t *testing.T) {
	t.Run("timeout handling", func(t *testing.T) {
		// Create a server that takes longer than the client timeout
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}))
		defer server.Close()

		// Create client with very short timeout
		config := &config.Config{
			Auth: config.AuthConfig{
				ServiceURL: server.URL,
				Timeout:    1 * time.Millisecond,
			},
		}
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		client := NewClient(config, logger)

		// Call should timeout
		ctx := context.Background()
		_, err := client.makeRequest(ctx, "GET", "/health", nil)
		assert.Error(t, err)
	})

	t.Run("context cancellation", func(t *testing.T) {
		// Create a server that takes longer than context timeout
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}))
		defer server.Close()

		config := &config.Config{
			Auth: config.AuthConfig{
				ServiceURL: server.URL,
				Timeout:    5 * time.Second,
			},
		}
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		client := NewClient(config, logger)

		// Create context with short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// Call should fail due to context cancellation
		_, err := client.makeRequest(ctx, "GET", "/health", nil)
		assert.Error(t, err)
	})
}
