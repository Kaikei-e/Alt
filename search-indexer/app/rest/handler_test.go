package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"search-indexer/domain"
	"search-indexer/logger"
	"search-indexer/usecase"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	logger.Init()
	os.Exit(m.Run())
}

// mockSearchEngine implements port.SearchEngine for testing
type mockSearchEngine struct {
	searchResult         []domain.SearchDocument
	searchErr            error
	searchByUserIDResult []domain.SearchDocument
	searchByUserIDErr    error

	// Recorded by SearchByUserIDWithDateFilter so tests can catch a
	// regression that silently drops the date window instead of forwarding
	// it to the search engine.
	dateFilterCalls     int
	gotDateFilterQuery  string
	gotDateFilterUserID string
	gotPublishedAfter   *time.Time
	gotPublishedBefore  *time.Time
}

func (m *mockSearchEngine) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	return nil
}
func (m *mockSearchEngine) DeleteDocuments(ctx context.Context, ids []string) error { return nil }
func (m *mockSearchEngine) SearchByUserIDWithDateFilter(ctx context.Context, query string, userID string, publishedAfter, publishedBefore *time.Time, limit int) ([]domain.SearchDocument, error) {
	m.dateFilterCalls++
	m.gotDateFilterQuery = query
	m.gotDateFilterUserID = userID
	m.gotPublishedAfter = publishedAfter
	m.gotPublishedBefore = publishedBefore
	return m.searchByUserIDResult, m.searchByUserIDErr
}
func (m *mockSearchEngine) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error) {
	return m.searchByUserIDResult, m.searchByUserIDErr
}
func (m *mockSearchEngine) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error) {
	return m.searchByUserIDResult, int64(len(m.searchByUserIDResult)), m.searchByUserIDErr
}
func (m *mockSearchEngine) EnsureIndex(ctx context.Context) error { return nil }
func (m *mockSearchEngine) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	return nil
}
func (m *mockSearchEngine) PruneTaskHistory(ctx context.Context, olderThan time.Duration) error {
	return nil
}

func TestHandler_SearchArticles(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		userID         string
		mockResults    []domain.SearchDocument
		mockErr        error
		wantStatusCode int
		wantHitCount   int
	}{
		{
			name:   "successful search",
			query:  "test",
			userID: "user1",
			mockResults: []domain.SearchDocument{
				{ID: "1", Title: "Test", Content: "Content", Tags: []string{"tag1"}},
			},
			wantStatusCode: http.StatusOK,
			wantHitCount:   1,
		},
		{
			name:           "missing query",
			query:          "",
			userID:         "user1",
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "missing user_id",
			query:          "test",
			userID:         "",
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "whitespace user_id",
			query:          "test",
			userID:         "   ",
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "search engine error",
			query:          "test",
			userID:         "user1",
			mockErr:        errors.New("search failed"),
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name:           "empty results",
			query:          "nonexistent",
			userID:         "user1",
			mockResults:    []domain.SearchDocument{},
			wantStatusCode: http.StatusOK,
			wantHitCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSearchEngine{
				searchByUserIDResult: tt.mockResults,
				searchByUserIDErr:    tt.mockErr,
			}

			searchByUserUsecase := usecase.NewSearchByUserUsecase(mock)
			handler := NewHandler(searchByUserUsecase)

			values := url.Values{}
			if tt.query != "" {
				values.Set("q", tt.query)
			}
			if tt.userID != "" {
				values.Set("user_id", tt.userID)
			}

			req := httptest.NewRequest(http.MethodGet, "/v1/search?"+values.Encode(), nil)
			rec := httptest.NewRecorder()

			handler.SearchArticles(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatusCode)
			}

			if tt.wantStatusCode == http.StatusOK {
				var resp SearchArticlesResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(resp.Hits) != tt.wantHitCount {
					t.Errorf("hit count = %d, want %d", len(resp.Hits), tt.wantHitCount)
				}
			}
		})
	}
}

func TestHandler_SearchArticles_WithoutUserID_ReturnsBadRequest(t *testing.T) {
	mock := &mockSearchEngine{}

	searchByUserUsecase := usecase.NewSearchByUserUsecase(mock)
	handler := NewHandler(searchByUserUsecase)

	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=iran+oil&limit=50", nil)
	rec := httptest.NewRecorder()

	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "user_id parameter required") {
		t.Errorf("body = %q, want containing 'user_id parameter required'", body)
	}
}

func TestHandler_SearchArticles_EmptyUserID_ReturnsBadRequest(t *testing.T) {
	mock := &mockSearchEngine{}

	searchByUserUsecase := usecase.NewSearchByUserUsecase(mock)
	handler := NewHandler(searchByUserUsecase)

	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=iran+oil&user_id=&limit=50", nil)
	rec := httptest.NewRecorder()

	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "user_id parameter required") {
		t.Errorf("body = %q, want containing 'user_id parameter required'", body)
	}
}

func TestHandler_SearchArticles_WhitespaceUserID_ReturnsBadRequest(t *testing.T) {
	mock := &mockSearchEngine{}

	searchByUserUsecase := usecase.NewSearchByUserUsecase(mock)
	handler := NewHandler(searchByUserUsecase)

	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=iran+oil&user_id=%20%20&limit=50", nil)
	rec := httptest.NewRecorder()

	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "user_id parameter required") {
		t.Errorf("body = %q, want containing 'user_id parameter required'", body)
	}
}

// TestHandler_SearchArticles_ResponseHasTotal verifies L-002: the REST
// response must expose a Total field so paginating clients know when to stop,
// matching the Connect-RPC SearchService contract's EstimatedTotalHits.
func TestHandler_SearchArticles_ResponseHasTotal(t *testing.T) {
	results := []domain.SearchDocument{
		{ID: "1", Title: "a", Content: "a", Tags: []string{}},
		{ID: "2", Title: "b", Content: "b", Tags: []string{}},
		{ID: "3", Title: "c", Content: "c", Tags: []string{}},
	}
	mock := &mockSearchEngine{
		searchResult:         results,
		searchByUserIDResult: results,
	}

	handler := NewHandler(
		usecase.NewSearchByUserUsecase(mock),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=test&user_id=u1", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp SearchArticlesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != len(results) {
		t.Fatalf("Total = %d, want %d", resp.Total, len(results))
	}
}
