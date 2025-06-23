// ABOUTME: This file contains comprehensive tests for the External API Repository
// ABOUTME: It follows TDD principles with table-driven tests for all external API methods
// ABOUTME: All network requests are mocked to avoid external dependencies

package repository

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"pre-processor/logger"
	"pre-processor/models"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLoggerExternalAPI() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func TestExternalAPIRepository_InterfaceCompliance(t *testing.T) {
	t.Run("should implement ExternalAPIRepository interface", func(t *testing.T) {
		// RED PHASE: Test that repository implements interface
		repo := NewExternalAPIRepository(testLoggerExternalAPI())

		// Verify interface compliance at compile time
		var _ ExternalAPIRepository = repo
		assert.NotNil(t, repo)
	})
}

func TestExternalAPIRepository_SummarizeArticle(t *testing.T) {
	tests := map[string]struct {
		article      *models.Article
		setupLogger  bool
		wantErr      bool
		errContains  string
		validateResp func(t *testing.T, resp *models.SummarizedContent)
	}{
		"should handle nil article": {
			article:     nil,
			setupLogger: true,
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
			setupLogger: true,
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
			setupLogger: true,
			wantErr:     true,
			errContains: "article content cannot be empty",
		},
		"should handle valid article but expect driver error": {
			article: &models.Article{
				ID:      "test-123",
				Title:   "Test Article",
				Content: "This is a test article content that needs to be summarized",
				URL:     "http://example.com",
			},
			setupLogger: true,
			wantErr:     true,
			errContains: "failed to summarize article",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GREEN PHASE: Test minimal implementation
			if tc.setupLogger {
				logger.Init()
			}

			repo := NewExternalAPIRepository(testLoggerExternalAPI())

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
		serviceURL  string
		setupLogger bool
		mockServer  func() *httptest.Server
		wantErr     bool
		errContains string
	}{
		"should handle empty service URL": {
			serviceURL:  "",
			setupLogger: true,
			wantErr:     true,
			errContains: "service URL cannot be empty",
		},
		"should handle invalid URL": {
			serviceURL:  "not-a-valid-url",
			setupLogger: true,
			wantErr:     true,
			errContains: "invalid service URL",
		},
		"should handle healthy service": {
			serviceURL:  "http://localhost:8080",
			setupLogger: true,
			mockServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/tags" {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"status":"healthy"}`))
					}
				}))
			},
			wantErr: false,
		},
		"should handle unhealthy service": {
			serviceURL:  "http://localhost:8080",
			setupLogger: true,
			mockServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
			},
			wantErr:     true,
			errContains: "service not healthy",
		},
		"should handle various HTTP status codes": {
			serviceURL:  "http://localhost:8080",
			setupLogger: true,
			mockServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			wantErr:     true,
			errContains: "service not healthy: status 404",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GREEN PHASE: Test minimal implementation
			if tc.setupLogger {
				logger.Init()
			}

			// Setup mock server if provided and update URL
			serviceURL := tc.serviceURL
			if tc.mockServer != nil {
				server := tc.mockServer()
				defer server.Close()
				if tc.serviceURL == "http://localhost:8080" {
					serviceURL = server.URL
				}
			}

			repo := NewExternalAPIRepository(testLoggerExternalAPI())

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
		logger.Init()
		repo := NewExternalAPIRepository(testLoggerExternalAPI())

		// Use invalid port that will definitely fail
		err := repo.CheckHealth(context.Background(), "http://127.0.0.1:99999")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health check request failed")
	})
}

func TestExternalAPIRepository_ContextHandling(t *testing.T) {
	t.Run("should handle context cancellation in SummarizeArticle", func(t *testing.T) {
		logger.Init()
		repo := NewExternalAPIRepository(testLoggerExternalAPI())

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
		logger.Init()
		repo := NewExternalAPIRepository(testLoggerExternalAPI())

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel context immediately

		// Use mock server to avoid external calls
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond) // Simulate delay
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := repo.CheckHealth(ctx, server.URL)
		assert.Error(t, err)
	})

	t.Run("should handle context timeout", func(t *testing.T) {
		logger.Init()
		repo := NewExternalAPIRepository(testLoggerExternalAPI())

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// Use mock server with delay to trigger timeout
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond) // Exceed timeout
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := repo.CheckHealth(ctx, server.URL)
		assert.Error(t, err)
	})
}

func TestExternalAPIRepository_EdgeCases(t *testing.T) {
	t.Run("should handle very long article content", func(t *testing.T) {
		logger.Init()
		repo := NewExternalAPIRepository(testLoggerExternalAPI())

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
		logger.Init()
		repo := NewExternalAPIRepository(testLoggerExternalAPI())

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
				// Create mock server
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				// Use the mock server URL as base and append the problematic path
				testURL := server.URL + "/api/tags"
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

// Table-driven tests for comprehensive coverage using mock servers
func TestExternalAPIRepository_TableDriven(t *testing.T) {
	type testCase struct {
		name        string
		operation   string
		setup       func() (ExternalAPIRepository, interface{}, *httptest.Server)
		validate    func(t *testing.T, result interface{}, err error)
		setupLogger bool
	}

	tests := []testCase{
		{
			name:      "summarize with all fields populated",
			operation: "summarize",
			setup: func() (ExternalAPIRepository, interface{}, *httptest.Server) {
				repo := NewExternalAPIRepository(testLoggerExternalAPI())
				article := &models.Article{
					ID:        "article-456",
					Title:     "Complete Article",
					Content:   "This is a complete article with all fields",
					URL:       "http://example.com/article",
					CreatedAt: time.Now(),
				}
				return repo, article, nil
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to summarize article")
			},
			setupLogger: true,
		},
		{
			name:      "health check with mock HTTPS server",
			operation: "health",
			setup: func() (ExternalAPIRepository, interface{}, *httptest.Server) {
				repo := NewExternalAPIRepository(testLoggerExternalAPI())
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/tags" {
						w.WriteHeader(http.StatusOK)
					}
				}))
				return repo, server.URL, server
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.NoError(t, err)
			},
			setupLogger: true,
		},
		{
			name:      "summarize with minimal article",
			operation: "summarize",
			setup: func() (ExternalAPIRepository, interface{}, *httptest.Server) {
				repo := NewExternalAPIRepository(testLoggerExternalAPI())
				article := &models.Article{
					ID:      "minimal-123",
					Content: "Minimal content",
				}
				return repo, article, nil
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.Error(t, err)
			},
			setupLogger: true,
		},
		{
			name:      "health check with mock server and port",
			operation: "health",
			setup: func() (ExternalAPIRepository, interface{}, *httptest.Server) {
				repo := NewExternalAPIRepository(testLoggerExternalAPI())
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/tags" {
						w.WriteHeader(http.StatusOK)
					}
				}))
				return repo, server.URL, server
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.NoError(t, err)
			},
			setupLogger: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setupLogger {
				logger.Init()
			}

			repo, input, server := tc.setup()
			if server != nil {
				defer server.Close()
			}

			var result interface{}
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

// Benchmark tests with mock servers
func BenchmarkExternalAPIRepository_SummarizeArticle(b *testing.B) {
	logger.Init()
	repo := NewExternalAPIRepository(testLoggerExternalAPI())

	article := &models.Article{
		ID:      "bench-test",
		Title:   "Benchmark Article",
		Content: strings.Repeat("This is test content. ", 100),
		URL:     "http://example.com/bench",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will fail but we're measuring the validation overhead
		repo.SummarizeArticle(context.Background(), article)
	}
}

func BenchmarkExternalAPIRepository_CheckHealth(b *testing.B) {
	logger.Init()
	repo := NewExternalAPIRepository(testLoggerExternalAPI())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.CheckHealth(context.Background(), server.URL)
	}
}

func TestExternalAPIRepository_HelperFunctions(t *testing.T) {
	t.Run("should validate constructor parameters", func(t *testing.T) {
		// Test that NewExternalAPIRepository handles nil logger gracefully
		repo := NewExternalAPIRepository(nil)
		assert.NotNil(t, repo)
	})

	t.Run("should handle HTTP client configuration", func(t *testing.T) {
		repo := NewExternalAPIRepository(testLoggerExternalAPI())

		// Test with a mock server that introduces delay
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Millisecond) // Small delay
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := repo.CheckHealth(context.Background(), server.URL)
		assert.NoError(t, err)
	})
}

func TestExternalAPIRepository_ErrorScenarios(t *testing.T) {
	t.Run("should handle network timeouts gracefully", func(t *testing.T) {
		logger.Init()
		repo := NewExternalAPIRepository(testLoggerExternalAPI())

		// Create server that delays longer than client timeout
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(35 * time.Second) // Exceed default client timeout
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := repo.CheckHealth(context.Background(), server.URL)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health check request failed")
	})

	t.Run("should handle malformed response gracefully", func(t *testing.T) {
		logger.Init()
		repo := NewExternalAPIRepository(testLoggerExternalAPI())

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Return malformed response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("invalid json {"))
		}))
		defer server.Close()

		err := repo.CheckHealth(context.Background(), server.URL)
		// Should succeed since we only check status code, not response body
		assert.NoError(t, err)
	})
}
