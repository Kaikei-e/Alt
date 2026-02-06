package usecase

import (
	"context"
	"search-indexer/domain"
	"testing"
	"time"
)

// Mock implementation for testing
type mockSearchEngine struct {
	indexedDocs []domain.SearchDocument
	err         error
}

func (m *mockSearchEngine) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	m.indexedDocs = docs
	return m.err
}

func (m *mockSearchEngine) DeleteDocuments(ctx context.Context, ids []string) error {
	// Remove deleted documents from indexedDocs
	filtered := []domain.SearchDocument{}
	for _, doc := range m.indexedDocs {
		found := false
		for _, id := range ids {
			if doc.ID == id {
				found = true
				break
			}
		}
		if !found {
			filtered = append(filtered, doc)
		}
	}
	m.indexedDocs = filtered
	return m.err
}

func (m *mockSearchEngine) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.indexedDocs, nil
}

func (m *mockSearchEngine) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.indexedDocs, nil
}

func (m *mockSearchEngine) EnsureIndex(ctx context.Context) error {
	return m.err
}

func (m *mockSearchEngine) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.indexedDocs, nil
}

func (m *mockSearchEngine) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.indexedDocs, int64(len(m.indexedDocs)), nil
}

func (m *mockSearchEngine) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	return m.err
}

func TestSearchArticlesUsecase_Execute(t *testing.T) {
	now := time.Now()
	article, _ := domain.NewArticle("1", "Test Title", "Test Content", []string{"tag1"}, now, "user1")
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
			mockErr:     &domain.SearchEngineError{Op: "Search", Err: "search failed"},
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

func TestSearchArticlesUsecase_ExecuteWithSecurityValidation(t *testing.T) {
	searchEngine := &mockSearchEngine{}
	usecase := NewSearchArticlesUsecase(searchEngine)

	tests := []struct {
		name    string
		query   string
		limit   int
		wantErr bool
	}{
		// XSS Prevention Tests
		{"script tag injection", "<script>alert('xss')</script>", 10, true},
		{"javascript protocol", "javascript:alert('xss')", 10, true},
		{"img tag with onerror", "<img src=x onerror=alert('xss')>", 10, true},
		{"iframe injection", "<iframe src=javascript:alert('xss')></iframe>", 10, true},
		{"svg with script", "<svg onload=alert('xss')>", 10, true},

		// SQL Injection Prevention Tests
		{"sql injection single quote", "'; DROP TABLE articles; --", 10, true},
		{"sql injection union", "test' UNION SELECT * FROM users--", 10, true},
		{"sql injection comment", "test/* comment */", 10, true},
		{"sql injection with semicolon", "test; DELETE FROM articles;", 10, true},

		// Command Injection Prevention Tests
		{"command injection pipe", "test | rm -rf /", 10, true},
		{"command injection semicolon", "test; cat /etc/passwd", 10, true},
		{"command injection backtick", "test`whoami`", 10, true},
		{"command injection dollar", "test$(whoami)", 10, true},

		// Control Character Tests
		{"null byte injection", "test\x00", 10, true},
		{"carriage return", "test\r\n", 10, true},
		{"vertical tab", "test\v", 10, true},
		{"form feed", "test\f", 10, true},

		// Unicode and Encoding Tests
		{"url encoded script", "%3Cscript%3Ealert%28%27xss%27%29%3C%2Fscript%3E", 10, true},
		{"double url encoded", "%253Cscript%253E", 10, true},
		{"zero width characters", "test\u200B\u200C\u200D", 10, true},

		// Valid queries that should pass
		{"normal search", "golang programming", 10, false},
		{"search with numbers", "python 3.11", 10, false},
		{"search with hyphens", "test-driven development", 10, false},
		{"search with spaces", "clean architecture", 10, false},
		{"search with unicode", "プログラミング", 10, false},
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
