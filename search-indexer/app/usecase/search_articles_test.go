package usecase

import (
	"context"
	"testing"
	"time"
	"search-indexer/domain"
	"search-indexer/port"
)

func TestSearchArticlesUsecase_Execute(t *testing.T) {
	now := time.Now()
	article, _ := domain.NewArticle("1", "Test Title", "Test Content", []string{"tag1"}, now)
	doc := domain.NewSearchDocument(article)

	tests := []struct {
		name        string
		query       string
		limit       int
		mockResults []domain.SearchDocument
		mockErr     error
		wantCount   int
		wantErr     bool
	}{
		{
			name:        "successful search",
			query:       "test",
			limit:       10,
			mockResults: []domain.SearchDocument{doc},
			mockErr:     nil,
			wantCount:   1,
			wantErr:     false,
		},
		{
			name:        "empty query",
			query:       "",
			limit:       10,
			mockResults: nil,
			mockErr:     nil,
			wantCount:   0,
			wantErr:     true,
		},
		{
			name:        "search engine error",
			query:       "test",
			limit:       10,
			mockResults: nil,
			mockErr:     &port.SearchEngineError{Op: "Search", Err: "search failed"},
			wantCount:   0,
			wantErr:     true,
		},
		{
			name:        "no results",
			query:       "nonexistent",
			limit:       10,
			mockResults: []domain.SearchDocument{},
			mockErr:     nil,
			wantCount:   0,
			wantErr:     false,
		},
		{
			name:        "limit validation",
			query:       "test",
			limit:       0,
			mockResults: nil,
			mockErr:     nil,
			wantCount:   0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchEngine := &mockSearchEngine{
				indexedDocs: tt.mockResults,
				err:         tt.mockErr,
			}

			usecase := NewSearchArticlesUsecase(searchEngine)
			
			result, err := usecase.Execute(context.Background(), tt.query, tt.limit)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(result.Documents) != tt.wantCount {
				t.Errorf("Execute() result count = %v, want %v", len(result.Documents), tt.wantCount)
			}

			if result.Query != tt.query {
				t.Errorf("Execute() result query = %v, want %v", result.Query, tt.query)
			}

			if result.Total != tt.wantCount {
				t.Errorf("Execute() result total = %v, want %v", result.Total, tt.wantCount)
			}
		})
	}
}

func TestSearchArticlesUsecase_ExecuteWithValidation(t *testing.T) {
	searchEngine := &mockSearchEngine{}
	usecase := NewSearchArticlesUsecase(searchEngine)

	tests := []struct {
		name    string
		query   string
		limit   int
		wantErr bool
	}{
		{"valid query and limit", "test", 10, false},
		{"empty query", "", 10, true},
		{"zero limit", "test", 0, true},
		{"negative limit", "test", -1, true},
		{"very long query", string(make([]byte, 1001)), 10, true},
		{"large limit", "test", 1001, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := usecase.Execute(context.Background(), tt.query, tt.limit)

			if tt.wantErr && err == nil {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}