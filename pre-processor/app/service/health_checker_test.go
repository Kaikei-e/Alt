package service

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLoggerHealth() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func TestHealthCheckerService_InterfaceCompliance(t *testing.T) {
	t.Run("should implement HealthCheckerService interface", func(t *testing.T) {
		// GREEN PHASE: Test that service implements interface
		service := NewHealthCheckerService("http://test:11434", testLoggerHealth())

		// Verify interface compliance at compile time
		var _ HealthCheckerService = service
		assert.NotNil(t, service)
	})
}

func TestHealthCheckerService_CheckNewsCreatorHealth(t *testing.T) {
	tests := map[string]struct {
		name         string
		mockResponse func(w http.ResponseWriter, r *http.Request)
		expectError  bool
		validateFunc func(t *testing.T, err error)
	}{
		"should handle healthy service": {
			mockResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"status":"healthy"}`))
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			},
			expectError: false,
		},
		"should handle unhealthy service": {
			mockResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			expectError: true,
			validateFunc: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "news creator not healthy")
			},
		},
		"should handle not found endpoint": {
			mockResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			expectError: true,
			validateFunc: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "news creator not healthy")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(tc.mockResponse))
			defer server.Close()

			service := NewHealthCheckerService(server.URL, testLoggerHealth())

			err := service.CheckNewsCreatorHealth(context.Background())

			if tc.expectError {
				require.Error(t, err)
				if tc.validateFunc != nil {
					tc.validateFunc(t, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}

	t.Run("should handle connection errors without external calls", func(t *testing.T) {
		// Use invalid port that will definitely fail
		service := NewHealthCheckerService("http://127.0.0.1:99999", testLoggerHealth())

		err := service.CheckNewsCreatorHealth(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health check failed")
	})
}

func TestHealthCheckerService_WaitForHealthy(t *testing.T) {
	t.Run("should handle cancelled context", func(t *testing.T) {
		// Create mock server that never responds healthy
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		service := NewHealthCheckerService(server.URL, testLoggerHealth())

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := service.WaitForHealthy(ctx)

		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("should return when service becomes healthy", func(t *testing.T) {
		// Create mock server that responds healthy
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/tags" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"healthy"}`))
			}
		}))
		defer server.Close()

		service := NewHealthCheckerService(server.URL, testLoggerHealth())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := service.WaitForHealthy(ctx)

		require.NoError(t, err)
	})

	t.Run("should handle timeout waiting for health", func(t *testing.T) {
		// Create mock server that never responds healthy
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		service := NewHealthCheckerService(server.URL, testLoggerHealth())

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := service.WaitForHealthy(ctx)

		assert.Error(t, err)
		assert.Equal(t, context.DeadlineExceeded, err)
	})
}
