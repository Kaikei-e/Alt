package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubHTTPClient struct {
	handler http.HandlerFunc
	err     error
	paths   []string
}

func (s *stubHTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.handler == nil {
		return nil, errors.New("handler not set")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	s.paths = append(s.paths, req.URL.Path)
	recorder := httptest.NewRecorder()
	s.handler(recorder, req)
	return recorder.Result(), nil
}

// newsCreatorStub serves the liveness-only /health body news-creator returns and
// the given deep-check response on /health/deep.
func newsCreatorStub(deepStatus int, deepBody string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"healthy","service":"news-creator"}`))
		case "/health/deep":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(deepStatus)
			if deepBody != "" {
				_, _ = w.Write([]byte(deepBody))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

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
		var _ = service

		assert.NotNil(t, service)
	})
}

func TestHealthCheckerService_CheckNewsCreatorHealth(t *testing.T) {
	tests := map[string]struct {
		mockResponse func(w http.ResponseWriter, r *http.Request)
		validateFunc func(t *testing.T, err error)
		expectError  bool
	}{
		"should be healthy when deep check passes": {
			mockResponse: newsCreatorStub(http.StatusOK,
				`{"status":"pass","service":"news-creator","checks":[{"name":"ollama","status":"pass","critical":true,"latency_ms":2}],"latency_ms":2,"cached":false}`),
			expectError: false,
		},
		"should be healthy when deep check warns": {
			mockResponse: newsCreatorStub(http.StatusOK,
				`{"status":"warn","service":"news-creator","checks":[{"name":"disk","status":"warn","critical":false,"latency_ms":1}],"latency_ms":1,"cached":false}`),
			expectError: false,
		},
		"should be unhealthy when deep check fails": {
			mockResponse: newsCreatorStub(http.StatusServiceUnavailable,
				`{"status":"fail","service":"news-creator","checks":[{"name":"ollama","status":"fail","critical":true,"latency_ms":5}],"latency_ms":5,"cached":false}`),
			expectError: true,
			validateFunc: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "news creator not healthy")
			},
		},
		"should be unhealthy when deep status is missing": {
			mockResponse: newsCreatorStub(http.StatusOK, `{"service":"news-creator","checks":[]}`),
			expectError:  true,
			validateFunc: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "news creator not healthy")
			},
		},
		"should be unhealthy when deep status is unknown": {
			mockResponse: newsCreatorStub(http.StatusOK, `{"status":"degraded","service":"news-creator"}`),
			expectError:  true,
			validateFunc: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "news creator not healthy")
			},
		},
		"should be unhealthy when liveness is up but deep endpoint is missing": {
			mockResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"status":"healthy","service":"news-creator"}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			},
			expectError: true,
			validateFunc: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "news creator not healthy")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			service := NewHealthCheckerService("http://news-creator.test", testLoggerHealth())
			stub := &stubHTTPClient{handler: tc.mockResponse}
			if concrete, ok := service.(*healthCheckerService); ok {
				concrete.client = stub
			}

			err := service.CheckNewsCreatorHealth(context.Background())

			assert.Equal(t, []string{"/health/deep"}, stub.paths, "health checker must probe the deep endpoint")

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
		service := NewHealthCheckerService("http://news-creator.test", testLoggerHealth())
		if concrete, ok := service.(*healthCheckerService); ok {
			concrete.client = &stubHTTPClient{err: errors.New("dial error")}
		}

		err := service.CheckNewsCreatorHealth(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health check failed")
	})
}

func TestHealthCheckerService_WaitForHealthy(t *testing.T) {
	t.Run("should handle canceled context", func(t *testing.T) {
		service := NewHealthCheckerService("http://news-creator.test", testLoggerHealth())
		if concrete, ok := service.(*healthCheckerService); ok {
			concrete.client = &stubHTTPClient{handler: newsCreatorStub(http.StatusServiceUnavailable,
				`{"status":"fail","service":"news-creator","checks":[{"name":"ollama","status":"fail","critical":true,"latency_ms":5}]}`)}
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := service.WaitForHealthy(ctx)

		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("should return when service becomes healthy", func(t *testing.T) {
		service := NewHealthCheckerService("http://news-creator.test", testLoggerHealth())
		stub := &stubHTTPClient{handler: newsCreatorStub(http.StatusOK,
			`{"status":"pass","service":"news-creator","checks":[{"name":"ollama","status":"pass","critical":true,"latency_ms":2}],"latency_ms":2,"cached":false}`)}
		if concrete, ok := service.(*healthCheckerService); ok {
			concrete.client = stub
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := service.WaitForHealthy(ctx)

		require.NoError(t, err)
		assert.Equal(t, []string{"/health/deep"}, stub.paths)
	})

	t.Run("should handle timeout waiting for health", func(t *testing.T) {
		service := NewHealthCheckerService("http://news-creator.test", testLoggerHealth())
		if concrete, ok := service.(*healthCheckerService); ok {
			concrete.client = &stubHTTPClient{handler: newsCreatorStub(http.StatusServiceUnavailable,
				`{"status":"fail","service":"news-creator","checks":[{"name":"ollama","status":"fail","critical":true,"latency_ms":5}]}`)}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := service.WaitForHealthy(ctx)

		assert.Error(t, err)
		assert.Equal(t, context.DeadlineExceeded, err)
	})
}
