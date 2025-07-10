package usecase

import (
	"context"
	"search-indexer/domain"
	"search-indexer/port"
	"testing"
	"time"
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

func TestSearchArticlesUsecase_SecurityValidation(t *testing.T) {
	searchEngine := &mockSearchEngine{}
	usecase := NewSearchArticlesUsecase(searchEngine)

	tests := []struct {
		name            string
		query           string
		limit           int
		expectError     bool
		expectSanitized bool
		expectedQuery   string
	}{
		{
			name:            "query with script injection",
			query:           "<script>alert('xss')</script>search term",
			limit:           10,
			expectError:     false,
			expectSanitized: true,
			expectedQuery:   "search term",
		},
		{
			name:            "query with HTML tags",
			query:           "<b>bold</b> text search",
			limit:           10,
			expectError:     false,
			expectSanitized: true,
			expectedQuery:   "bold text search",
		},
		{
			name:            "query with dangerous characters",
			query:           "search'; DROP TABLE users; --",
			limit:           10,
			expectError:     true,
			expectSanitized: false,
			expectedQuery:   "",
		},
		{
			name:            "query with SQL injection attempt",
			query:           "1' OR '1'='1",
			limit:           10,
			expectError:     true,
			expectSanitized: false,
			expectedQuery:   "",
		},
		{
			name:            "query with javascript protocol",
			query:           "javascript:alert('xss')",
			limit:           10,
			expectError:     false,
			expectSanitized: true,
			expectedQuery:   "",
		},
		{
			name:            "query with data protocol",
			query:           "data:text/html,<script>alert('xss')</script>",
			limit:           10,
			expectError:     false,
			expectSanitized: true,
			expectedQuery:   "text/html,",
		},
		{
			name:            "query with event handlers",
			query:           "onload=alert('xss') search",
			limit:           10,
			expectError:     false,
			expectSanitized: true,
			expectedQuery:   " search",
		},
		{
			name:            "query with multiple dangerous chars",
			query:           "search<>\"'\\/*;",
			limit:           10,
			expectError:     true,
			expectSanitized: false,
			expectedQuery:   "",
		},
		{
			name:            "query with allowed special chars",
			query:           "go-lang & programming!",
			limit:           10,
			expectError:     false,
			expectSanitized: false,
			expectedQuery:   "go-lang & programming!",
		},
		{
			name:            "query with excessive whitespace",
			query:           "   multiple    spaces   between   words   ",
			limit:           10,
			expectError:     false,
			expectSanitized: true,
			expectedQuery:   "multiple spaces between words",
		},
		{
			name:            "extremely long query",
			query:           string(make([]byte, 1001)),
			limit:           10,
			expectError:     true,
			expectSanitized: false,
			expectedQuery:   "",
		},
		{
			name:            "query at maximum length",
			query:           string(make([]byte, 1000)),
			limit:           10,
			expectError:     false,
			expectSanitized: false,
			expectedQuery:   string(make([]byte, 1000)),
		},
		{
			name:            "malformed HTML query",
			query:           "<div>unclosed tag content",
			limit:           10,
			expectError:     false,
			expectSanitized: true,
			expectedQuery:   "",
		},
		{
			name:            "nested script tags",
			query:           "<script><script>alert('nested')</script></script>",
			limit:           10,
			expectError:     false,
			expectSanitized: true,
			expectedQuery:   "",
		},
		{
			name:            "case insensitive script detection",
			query:           "<SCRIPT>ALERT('XSS')</SCRIPT>search",
			limit:           10,
			expectError:     false,
			expectSanitized: true,
			expectedQuery:   "search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := usecase.Execute(context.Background(), tt.query, tt.limit)

			if tt.expectError {
				if err == nil {
					t.Errorf("Execute() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Execute() unexpected error: %v", err)
				return
			}

			if tt.expectSanitized {
				if result.Query != tt.expectedQuery {
					t.Errorf("Execute() sanitized query = %q, want %q", result.Query, tt.expectedQuery)
				}
			} else {
				if result.Query != tt.query {
					t.Errorf("Execute() query = %q, want %q", result.Query, tt.query)
				}
			}
		})
	}
}

func TestSearchArticlesUsecase_SecurityBoundaryConditions(t *testing.T) {
	searchEngine := &mockSearchEngine{}
	usecase := NewSearchArticlesUsecase(searchEngine)

	tests := []struct {
		name        string
		query       string
		limit       int
		expectError bool
		description string
	}{
		{
			name:        "query with null byte",
			query:       "search\x00term",
			limit:       10,
			expectError: true,
			description: "null byte injection attempt",
		},
		{
			name:        "query with unicode control characters",
			query:       "search\u0000\u0001\u0002term",
			limit:       10,
			expectError: true,
			description: "unicode control character injection",
		},
		{
			name:        "query with CRLF injection",
			query:       "search\r\nterm",
			limit:       10,
			expectError: false,
			description: "CRLF should be normalized to space",
		},
		{
			name:        "query with tab characters",
			query:       "search\tterm",
			limit:       10,
			expectError: false,
			description: "tabs should be normalized to space",
		},
		{
			name:        "query with mixed encodings",
			query:       "search%3Cscript%3Ealert('xss')%3C/script%3E",
			limit:       10,
			expectError: false,
			description: "URL encoded script tags should be handled",
		},
		{
			name:        "query with zero-width characters",
			query:       "search\u200B\u200C\u200D\uFEFFterm",
			limit:       10,
			expectError: false,
			description: "zero-width characters should be handled",
		},
		{
			name:        "query with mathematical symbols",
			query:       "search ∀ ∃ ∈ ∉ ∧ ∨ ¬ ⊕ term",
			limit:       10,
			expectError: false,
			description: "mathematical symbols should be allowed",
		},
		{
			name:        "query with emoji",
			query:       "search 🔍 📝 💻 term",
			limit:       10,
			expectError: false,
			description: "emoji should be allowed",
		},
		{
			name:        "query with international characters",
			query:       "search 日本語 中文 한국어 العربية русский",
			limit:       10,
			expectError: false,
			description: "international characters should be allowed",
		},
		{
			name:        "query with mixed case script tags",
			query:       "<ScRiPt>AlErT('XsS')</ScRiPt>",
			limit:       10,
			expectError: false,
			description: "case insensitive script tag detection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := usecase.Execute(context.Background(), tt.query, tt.limit)

			if tt.expectError && err == nil {
				t.Errorf("Execute() expected error for %s but got none", tt.description)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Execute() unexpected error for %s: %v", tt.description, err)
			}
		})
	}
}
