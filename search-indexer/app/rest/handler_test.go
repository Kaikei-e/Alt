package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"search-indexer/domain"
	"search-indexer/logger"
	"search-indexer/usecase"
	"testing"
)

func TestMain(m *testing.M) {
	logger.Init()
	os.Exit(m.Run())
}

// mockSearchEngine implements port.SearchEngine for testing
type mockSearchEngine struct {
	searchByUserIDResult []domain.SearchDocument
	searchByUserIDErr    error
}

func (m *mockSearchEngine) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	return nil
}
func (m *mockSearchEngine) DeleteDocuments(ctx context.Context, ids []string) error { return nil }
func (m *mockSearchEngine) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (m *mockSearchEngine) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
	return nil, nil
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

			url := "/v1/search?"
			if tt.query != "" {
				url += "q=" + tt.query + "&"
			}
			if tt.userID != "" {
				url += "user_id=" + tt.userID
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
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
