package search_indexer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"alt/orchestrator/driver/models"
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

// The tests below exercise doSearchRequest directly (this file lives in the
// same package as api.go) rather than SearchArticles/SearchArticlesWithUserID,
// because doSearchRequest already accepts an arbitrary target endpoint - no
// production refactor is needed to point it at an httptest server.

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

	target, err := BuildSearchURL(server.URL, "/v1/search", "test")
	if err != nil {
		t.Fatalf("BuildSearchURL() unexpected error: %v", err)
	}

	hits, err := doSearchRequest(context.Background(), target)
	if err != nil {
		t.Fatalf("doSearchRequest() unexpected error: %v", err)
	}

	want := []models.SearchArticlesHit{
		{ID: "1", Title: "Test Article", Content: "Content 1"},
		{ID: "2", Title: "Another Article", Content: "Content 2"},
	}
	if len(hits) != len(want) {
		t.Fatalf("doSearchRequest() returned %d hits, want %d", len(hits), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(hits[i], want[i]) {
			t.Errorf("doSearchRequest() hit[%d] = %+v, want %+v", i, hits[i], want[i])
		}
	}
}

func TestSearchArticles_ServiceUnavailable(t *testing.T) {
	// Start and immediately close a server so the connection is refused,
	// exercising the isConnectionError -> ErrSearchServiceUnavailable path.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	target := server.URL + "/v1/search?q=test"
	server.Close()

	hits, err := doSearchRequest(context.Background(), target)
	if !errors.Is(err, ErrSearchServiceUnavailable) {
		t.Errorf("doSearchRequest() error = %v, want ErrSearchServiceUnavailable", err)
	}
	if hits != nil {
		t.Errorf("doSearchRequest() hits = %v, want nil on error", hits)
	}
}

func TestSearchArticles_Timeout(t *testing.T) {
	// Server responds slower than the caller's context deadline, exercising
	// the isTimeoutError -> ErrSearchTimeout path without waiting on the
	// package's full 10s http.Client timeout.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	hits, err := doSearchRequest(ctx, server.URL+"/v1/search?q=test")
	if !errors.Is(err, ErrSearchTimeout) {
		t.Errorf("doSearchRequest() error = %v, want ErrSearchTimeout", err)
	}
	if hits != nil {
		t.Errorf("doSearchRequest() hits = %v, want nil on error", hits)
	}
}

func TestSearchArticles_Non200Status(t *testing.T) {
	// Test when search-indexer returns non-200 status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	hits, err := doSearchRequest(context.Background(), server.URL+"/v1/search?q=test")
	if err == nil {
		t.Fatal("doSearchRequest() expected error for non-200 status, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("doSearchRequest() error = %v, want it to mention status 500", err)
	}
	if hits != nil {
		t.Errorf("doSearchRequest() hits = %v, want nil on error", hits)
	}
}

func TestSearchArticles_InvalidJSON(t *testing.T) {
	// Test when search-indexer returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	hits, err := doSearchRequest(context.Background(), server.URL+"/v1/search?q=test")
	if err == nil {
		t.Fatal("doSearchRequest() expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("doSearchRequest() error = %v, want it to mention unmarshal failure", err)
	}
	if hits != nil {
		t.Errorf("doSearchRequest() hits = %v, want nil on error", hits)
	}
}

// TestSearchArticlesWithUserID tests are similar to TestSearchArticles
// but include user_id parameter validation

func TestSearchArticlesWithUserID_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("user_id"); got != "user-123" {
			t.Errorf("Expected user_id=user-123, got %q", got)
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

	target, err := BuildSearchURLWithUserID(server.URL, "/v1/search", "test", "user-123")
	if err != nil {
		t.Fatalf("BuildSearchURLWithUserID() unexpected error: %v", err)
	}

	hits, err := doSearchRequest(context.Background(), target)
	if err != nil {
		t.Fatalf("doSearchRequest() unexpected error: %v", err)
	}
	want := []models.SearchArticlesHit{{ID: "1", Title: "Test Article", Content: "Content 1"}}
	if len(hits) != len(want) || !reflect.DeepEqual(hits[0], want[0]) {
		t.Errorf("doSearchRequest() hits = %+v, want %+v", hits, want)
	}
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
