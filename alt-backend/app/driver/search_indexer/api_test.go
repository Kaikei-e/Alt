package search_indexer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alt/driver/models"
)

func TestBuildSearchURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		path     string
		query    string
		expected string
		wantErr  bool
	}{
		{
			name:     "valid URL with simple query",
			baseURL:  "http://localhost:9300",
			path:     "/v1/search",
			query:    "test",
			expected: "http://localhost:9300/v1/search?q=test",
			wantErr:  false,
		},
		{
			name:     "URL encodes special characters",
			baseURL:  "http://localhost:9300",
			path:     "/v1/search",
			query:    "hello world",
			expected: "http://localhost:9300/v1/search?q=hello+world",
			wantErr:  false,
		},
		{
			name:     "handles Japanese characters",
			baseURL:  "http://localhost:9300",
			path:     "/v1/search",
			query:    "テスト",
			expected: "http://localhost:9300/v1/search?q=%E3%83%86%E3%82%B9%E3%83%88",
			wantErr:  false,
		},
		{
			name:    "invalid base URL",
			baseURL: "://invalid",
			path:    "/v1/search",
			query:   "test",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildSearchURL(tt.baseURL, tt.path, tt.query)
			if tt.wantErr {
				if err == nil {
					t.Errorf("BuildSearchURL() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("BuildSearchURL() unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("BuildSearchURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBuildSearchURLWithUserID(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		path     string
		query    string
		userID   string
		expected string
		wantErr  bool
	}{
		{
			name:     "valid URL with user ID",
			baseURL:  "http://localhost:9300",
			path:     "/v1/search",
			query:    "test",
			userID:   "user-123",
			expected: "http://localhost:9300/v1/search?q=test&user_id=user-123",
			wantErr:  false,
		},
		{
			name:     "UUID user ID",
			baseURL:  "http://localhost:9300",
			path:     "/v1/search",
			query:    "test",
			userID:   "550e8400-e29b-41d4-a716-446655440000",
			expected: "http://localhost:9300/v1/search?q=test&user_id=550e8400-e29b-41d4-a716-446655440000",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildSearchURLWithUserID(tt.baseURL, tt.path, tt.query, tt.userID)
			if tt.wantErr {
				if err == nil {
					t.Errorf("BuildSearchURLWithUserID() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("BuildSearchURLWithUserID() unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("BuildSearchURLWithUserID() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSearchArticles_Success(t *testing.T) {
	// Create a test server that returns valid search results
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search" {
			t.Errorf("Expected path /v1/search, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "test" {
			t.Errorf("Expected query 'test', got %s", r.URL.Query().Get("q"))
		}

		response := models.SearchArticlesAPIResponse{
			Hits: []models.SearchArticlesHit{
				{ID: "1", Title: "Test Article", Content: "Content 1"},
				{ID: "2", Title: "Another Article", Content: "Content 2"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// We need to modify the function to accept a custom host/port or use dependency injection
	// For now, this test documents the expected behavior
	// TODO: Refactor to allow testable HTTP client injection
	t.Skip("Requires refactoring to inject test server URL")
}

func TestSearchArticles_ServiceUnavailable(t *testing.T) {
	// Test when search-indexer service is not available
	// This should return ErrSearchServiceUnavailable
	t.Skip("Requires refactoring to inject test server URL and implementing ErrSearchServiceUnavailable")
}

func TestSearchArticles_Timeout(t *testing.T) {
	// Create a test server that delays response beyond timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // Longer than 10s timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// TODO: Refactor to allow testable HTTP client injection
	t.Skip("Requires refactoring to inject test server URL and implementing ErrSearchTimeout")
}

func TestSearchArticles_Non200Status(t *testing.T) {
	// Test when search-indexer returns non-200 status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	// TODO: Refactor to allow testable HTTP client injection
	t.Skip("Requires refactoring to inject test server URL")
}

func TestSearchArticles_InvalidJSON(t *testing.T) {
	// Test when search-indexer returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	// TODO: Refactor to allow testable HTTP client injection
	t.Skip("Requires refactoring to inject test server URL")
}

// TestSearchArticlesWithUserID tests are similar to TestSearchArticles
// but include user_id parameter validation

func TestSearchArticlesWithUserID_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("user_id") == "" {
			t.Errorf("Expected user_id parameter, got none")
		}

		response := models.SearchArticlesAPIResponse{
			Hits: []models.SearchArticlesHit{
				{ID: "1", Title: "Test Article", Content: "Content 1"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	t.Skip("Requires refactoring to inject test server URL")
}

// Test for specific error types that should be added
func TestErrorTypes(t *testing.T) {
	// These tests verify that specific error types exist
	// They will fail until the error types are implemented

	t.Run("ErrSearchServiceUnavailable exists", func(t *testing.T) {
		if ErrSearchServiceUnavailable == nil {
			t.Error("ErrSearchServiceUnavailable should be defined")
		}
	})

	t.Run("ErrSearchTimeout exists", func(t *testing.T) {
		if ErrSearchTimeout == nil {
			t.Error("ErrSearchTimeout should be defined")
		}
	})
}
