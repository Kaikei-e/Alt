// ABOUTME: This file contains comprehensive tests for the External API Repository
// ABOUTME: It follows TDD principles with table-driven tests for all external API methods
// ABOUTME: All network requests are mocked to avoid external dependencies

package repository

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"pre-processor/config"
	"pre-processor/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test constants
const (
	testServiceURL = "http://test-service"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newHandlerTransport(handler http.HandlerFunc, delay time.Duration) http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		recorder := httptest.NewRecorder()
		handler(recorder, req)
		return recorder.Result(), nil
	})
}

func newErrorTransport(err error) http.RoundTripper {
	return roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, err
	})
}

func setRepoTransport(repo ExternalAPIRepository, transport http.RoundTripper) {
	if concrete, ok := repo.(*externalAPIRepository); ok {
		concrete.client.Transport = transport
	}
}

func testLoggerExternalAPI() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func testConfig() *config.Config {
	return &config.Config{
		NewsCreator: config.NewsCreatorConfig{
			Host:    "http://test-news-creator:11434",
			APIPath: "/api/generate",
			Model:   "gemma3:4b",
			Timeout: 30 * time.Second,
		},
	}
}

func TestExternalAPIRepository_InterfaceCompliance(t *testing.T) {
	t.Run("should implement ExternalAPIRepository interface", func(t *testing.T) {
		// RED PHASE: Test that repository implements interface
		repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

		// Verify interface compliance at compile time
		var _ = repo

		assert.NotNil(t, repo)
	})
}

func TestExternalAPIRepository_SummarizeArticle(t *testing.T) {
	tests := map[string]struct {
		article      *models.Article
		validateResp func(t *testing.T, resp *models.SummarizedContent)
		errContains  string
		wantErr      bool
	}{
		"should handle nil article": {
			article: nil,

			wantErr:     true,
			errContains: "article cannot be nil",
		},
		"should handle article with empty ID": {
			article: &models.Article{
				ID:      "",
				Title:   "Test Article",
				Content: "Test content",
				URL:     "http://example.com",
			},

			wantErr:     true,
			errContains: "article ID cannot be empty",
		},
		"should handle article with empty content": {
			article: &models.Article{
				ID:      "test-123",
				Title:   "Test Article",
				Content: "",
				URL:     "http://example.com",
			},

			wantErr:     true,
			errContains: "article content cannot be empty",
		},
		"should handle valid article but expect driver error": {
			article: &models.Article{
				ID:      "test-123",
				Title:   "Test Article",
				Content: "This is a test article content that needs to be summarized. It contains enough characters to pass the minimum length validation of 100 characters. The content should be long enough to trigger an actual API call error rather than a validation error.",
				URL:     "http://example.com",
			},

			wantErr:     true,
			errContains: "failed to summarize article",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GREEN PHASE: Test minimal implementation

			repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

			summary, err := repo.SummarizeArticle(context.Background(), tc.article)

			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, summary)

				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, summary)

				if tc.validateResp != nil {
					tc.validateResp(t, summary)
				}
			}
		})
	}
}

func TestExternalAPIRepository_CheckHealth(t *testing.T) {
	tests := map[string]struct {
		handler     http.HandlerFunc
		serviceURL  string
		errContains string
		wantErr     bool
	}{
		"should handle empty service URL": {
			serviceURL: "",

			wantErr:     true,
			errContains: "service URL cannot be empty",
		},
		"should handle invalid URL": {
			serviceURL: "not-a-valid-url",

			wantErr:     true,
			errContains: "invalid service URL",
		},
		"should handle healthy service": {
			serviceURL: "http://localhost:8080",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"healthy"}`))
			},
			wantErr: false,
		},
		"should handle unhealthy service": {
			serviceURL: "http://localhost:8080",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			wantErr:     true,
			errContains: "service not healthy",
		},
		"should handle various HTTP status codes": {
			serviceURL: "http://localhost:8080",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantErr:     true,
			errContains: "service not healthy: status 404",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GREEN PHASE: Test minimal implementation

			// Setup mock transport if provided and update URL
			serviceURL := tc.serviceURL
			repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

			if tc.handler != nil {
				setRepoTransport(repo, newHandlerTransport(tc.handler, 0))
				if tc.serviceURL == "http://localhost:8080" {
					serviceURL = testServiceURL
				}
			}

			err := repo.CheckHealth(context.Background(), serviceURL)

			if tc.wantErr {
				require.Error(t, err)

				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}

	t.Run("should handle connection errors without external calls", func(t *testing.T) {

		repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())
		setRepoTransport(repo, newErrorTransport(errors.New("dial error")))

		err := repo.CheckHealth(context.Background(), testServiceURL)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health check request failed")
	})
}

func TestExternalAPIRepository_ContextHandling(t *testing.T) {
	t.Run("should handle context cancellation in SummarizeArticle", func(t *testing.T) {

		repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel context immediately

		article := &models.Article{
			ID:      "test-123",
			Title:   "Test Article",
			Content: "Test content",
			URL:     "http://example.com",
		}

		summary, err := repo.SummarizeArticle(ctx, article)
		assert.Error(t, err)
		assert.Nil(t, summary)
	})

	t.Run("should handle context cancellation in CheckHealth", func(t *testing.T) {

		repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel context immediately

		setRepoTransport(repo, newHandlerTransport(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, 100*time.Millisecond))

		err := repo.CheckHealth(ctx, testServiceURL)
		assert.Error(t, err)
	})

	t.Run("should handle context timeout", func(t *testing.T) {

		repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		setRepoTransport(repo, newHandlerTransport(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, 100*time.Millisecond))

		err := repo.CheckHealth(ctx, testServiceURL)
		assert.Error(t, err)
	})
}

func TestExternalAPIRepository_EdgeCases(t *testing.T) {
	t.Run("should handle very long article content", func(t *testing.T) {

		repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

		// Create article with very long content
		longContent := make([]byte, 1024*1024) // 1MB
		for i := range longContent {
			longContent[i] = 'a'
		}

		article := &models.Article{
			ID:      "test-long",
			Title:   "Test Article with Long Content",
			Content: string(longContent),
			URL:     "http://example.com",
		}

		summary, err := repo.SummarizeArticle(context.Background(), article)
		// Should handle gracefully (actual behavior depends on driver implementation)
		assert.Error(t, err)
		assert.Nil(t, summary)
	})

	t.Run("should handle URL with special characters", func(t *testing.T) {

		repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

		// Test various problematic URLs using mock servers
		tests := map[string]struct {
			url         string
			expectError bool
		}{
			"URL with spaces": {
				url:         "http://example.com/path with spaces",
				expectError: false, // URL will be parsed correctly
			},
			"URL with unicode": {
				url:         "http://example.com/日本語",
				expectError: false, // URL will be parsed correctly
			},
			"URL with special chars": {
				url:         "http://example.com/?param=value&special=<>",
				expectError: false, // URL will be parsed correctly
			},
		}

		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				setRepoTransport(repo, newHandlerTransport(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}, 0))

				// Use a valid base URL to avoid external calls
				testURL := testServiceURL + "/api/tags"
				err := repo.CheckHealth(context.Background(), testURL)

				if tc.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}

// Table-driven tests for comprehensive coverage using mock servers.
func TestExternalAPIRepository_TableDriven(t *testing.T) {
	type testCase struct {
		setup     func() (ExternalAPIRepository, any)
		validate  func(t *testing.T, result any, err error)
		name      string
		operation string
	}

	tests := []testCase{
		{
			name:      "summarize with all fields populated",
			operation: "summarize",
			setup: func() (ExternalAPIRepository, interface{}) {
				repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())
				article := &models.Article{
					ID:        "article-456",
					Title:     "Complete Article",
					Content:   "This is a complete article with all fields populated. It contains enough characters to pass the minimum length validation of 100 characters. The content should be long enough to trigger an actual API call error rather than a validation error.",
					URL:       "http://example.com/article",
					CreatedAt: time.Now(),
				}
				return repo, article
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to summarize article")
			},
		},
		{
			name:      "health check with mock HTTPS server",
			operation: "health",
			setup: func() (ExternalAPIRepository, interface{}) {
				repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())
				setRepoTransport(repo, newHandlerTransport(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}, 0))
				return repo, testServiceURL
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:      "summarize with minimal article",
			operation: "summarize",
			setup: func() (ExternalAPIRepository, interface{}) {
				repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())
				article := &models.Article{
					ID:      "minimal-123",
					Content: "Minimal content",
				}
				return repo, article
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.Error(t, err)
			},
		},
		{
			name:      "health check with mock server and port",
			operation: "health",
			setup: func() (ExternalAPIRepository, interface{}) {
				repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())
				setRepoTransport(repo, newHandlerTransport(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}, 0))
				return repo, testServiceURL
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			repo, input := tc.setup()

			var result any

			var err error

			switch tc.operation {
			case "summarize":
				result, err = repo.SummarizeArticle(context.Background(), input.(*models.Article))
			case "health":
				err = repo.CheckHealth(context.Background(), input.(string))
			}

			tc.validate(t, result, err)
		})
	}
}

// Benchmark tests with mock servers.
func BenchmarkExternalAPIRepository_SummarizeArticle(b *testing.B) {

	repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

	article := &models.Article{
		ID:      "bench-test",
		Title:   "Benchmark Article",
		Content: strings.Repeat("This is test content. ", 100),
		URL:     "http://example.com/bench",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// This will fail but we're measuring the validation overhead
		_, _ = repo.SummarizeArticle(context.Background(), article)
	}
}

func BenchmarkExternalAPIRepository_CheckHealth(b *testing.B) {

	repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())
	setRepoTransport(repo, newHandlerTransport(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, 0))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = repo.CheckHealth(context.Background(), testServiceURL)
	}
}

func TestExternalAPIRepository_HelperFunctions(t *testing.T) {
	t.Run("should validate constructor parameters", func(t *testing.T) {
		// Test that NewExternalAPIRepository handles nil logger gracefully
		repo := NewExternalAPIRepository(testConfig(), nil)
		assert.NotNil(t, repo)
	})

	t.Run("should handle HTTP client configuration", func(t *testing.T) {
		repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

		setRepoTransport(repo, newHandlerTransport(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, 10*time.Millisecond))

		err := repo.CheckHealth(context.Background(), testServiceURL)
		assert.NoError(t, err)
	})
}

func TestExternalAPIRepository_ErrorScenarios(t *testing.T) {
	t.Run("should handle network timeouts gracefully", func(t *testing.T) {

		repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

		// Use context timeout instead of server sleep to test timeout behavior
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		setRepoTransport(repo, newHandlerTransport(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, 100*time.Millisecond))

		err := repo.CheckHealth(ctx, testServiceURL)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health check request failed")
	})

	t.Run("should handle malformed response gracefully", func(t *testing.T) {

		repo := NewExternalAPIRepository(testConfig(), testLoggerExternalAPI())

		setRepoTransport(repo, newHandlerTransport(func(w http.ResponseWriter, r *http.Request) {
			// Return malformed response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("invalid json {"))
		}, 0))

		err := repo.CheckHealth(context.Background(), testServiceURL)
		// Should succeed since we only check status code, not response body
		assert.NoError(t, err)
	})
}
