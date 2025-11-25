package usecase

import (
	"context"
	"errors"
	"search-indexer/domain"
	"testing"
)

// MockSearchEngine for testing
type MockSearchEngine struct {
	searchWithFiltersFunc func(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error)
}

func (m *MockSearchEngine) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	return nil
}

func (m *MockSearchEngine) DeleteDocuments(ctx context.Context, ids []string) error {
	return nil
}

func (m *MockSearchEngine) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	return nil, nil
}

func (m *MockSearchEngine) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
	if m.searchWithFiltersFunc != nil {
		return m.searchWithFiltersFunc(ctx, query, filters, limit)
	}
	return nil, nil
}

func (m *MockSearchEngine) EnsureIndex(ctx context.Context) error {
	return nil
}

func (m *MockSearchEngine) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	return nil
}

func TestSearchArticlesWithFiltersUsecase_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		query           string
		filters         []string
		limit           int
		searchEngineErr error
		expectedErr     string
		expectedResults []domain.SearchDocument
	}{
		{
			name:    "successful search with filters",
			query:   "technology",
			filters: []string{"programming", "web-development"},
			limit:   10,
			expectedResults: []domain.SearchDocument{
				{
					ID:      "1",
					Title:   "Test Article",
					Content: "Test content about technology and programming",
					Tags:    []string{"programming", "web-development"},
				},
			},
		},
		{
			name:    "successful search with no filters",
			query:   "technology",
			filters: []string{},
			limit:   10,
			expectedResults: []domain.SearchDocument{
				{
					ID:      "1",
					Title:   "Test Article",
					Content: "Test content about technology",
					Tags:    []string{},
				},
			},
		},
		{
			name:        "empty query",
			query:       "",
			filters:     []string{"programming"},
			limit:       10,
			expectedErr: "query cannot be empty",
		},
		{
			name:        "whitespace only query",
			query:       "   ",
			filters:     []string{"programming"},
			limit:       10,
			expectedErr: "query cannot be empty",
		},
		{
			name:        "query too long",
			query:       string(make([]byte, 1001)),
			filters:     []string{"programming"},
			limit:       10,
			expectedErr: "query too long: maximum 1000 characters, got 1001",
		},
		{
			name:        "negative limit",
			query:       "technology",
			filters:     []string{"programming"},
			limit:       -1,
			expectedErr: "limit must be positive: got -1",
		},
		{
			name:        "zero limit",
			query:       "technology",
			filters:     []string{"programming"},
			limit:       0,
			expectedErr: "limit must be positive: got 0",
		},
		{
			name:        "limit too large",
			query:       "technology",
			filters:     []string{"programming"},
			limit:       101,
			expectedErr: "limit too large: maximum 100, got 101",
		},
		{
			name:        "too many filters",
			query:       "technology",
			filters:     make([]string, 11),
			limit:       10,
			expectedErr: "too many filters: maximum 10, got 11",
		},
		{
			name:            "search engine error",
			query:           "technology",
			filters:         []string{"programming"},
			limit:           10,
			searchEngineErr: errors.New("search engine failed"),
			expectedErr:     "search with filters failed: search engine failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSearchEngine := &MockSearchEngine{
				searchWithFiltersFunc: func(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
					if tt.searchEngineErr != nil {
						return nil, tt.searchEngineErr
					}
					return tt.expectedResults, nil
				},
			}

			usecase := NewSearchArticlesWithFiltersUsecase(mockSearchEngine)
			results, err := usecase.Execute(ctx, tt.query, tt.filters, tt.limit)

			if tt.expectedErr != "" {
				if err == nil {
					t.Errorf("expected error %q, got nil", tt.expectedErr)
				} else if err.Error() != tt.expectedErr {
					t.Errorf("expected error %q, got %q", tt.expectedErr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %q", err.Error())
				}
				if len(results) != len(tt.expectedResults) {
					t.Errorf("expected %d results, got %d", len(tt.expectedResults), len(results))
				}
			}
		})
	}
}

func TestSearchArticlesWithFiltersUsecase_SecurityValidation(t *testing.T) {
	ctx := context.Background()

	securityTests := []struct {
		name        string
		query       string
		filters     []string
		limit       int
		expectError bool
		description string
	}{
		{
			name:        "malicious filter with script tags",
			query:       "technology",
			filters:     []string{"<script>alert('xss')</script>"},
			limit:       10,
			expectError: true,
			description: "Should reject filters with script tags",
		},
		{
			name:        "malicious filter with SQL injection",
			query:       "technology",
			filters:     []string{"'; DROP TABLE articles; --"},
			limit:       10,
			expectError: true,
			description: "Should reject filters with SQL injection",
		},
		{
			name:        "malicious filter with Meilisearch injection",
			query:       "technology",
			filters:     []string{"tag\" OR \"admin"},
			limit:       10,
			expectError: true,
			description: "Should reject filters with Meilisearch injection",
		},
		{
			name:        "valid filter with unicode",
			query:       "technology",
			filters:     []string{"テクノロジー"},
			limit:       10,
			expectError: false,
			description: "Should accept valid unicode filters",
		},
		{
			name:        "valid filter with spaces",
			query:       "technology",
			filters:     []string{"machine learning"},
			limit:       10,
			expectError: false,
			description: "Should accept valid filters with spaces",
		},
		{
			name:        "filter too long",
			query:       "technology",
			filters:     []string{string(make([]byte, 101))},
			limit:       10,
			expectError: true,
			description: "Should reject filters that are too long",
		},
		{
			name:        "empty filter",
			query:       "technology",
			filters:     []string{""},
			limit:       10,
			expectError: true,
			description: "Should reject empty filters",
		},
		{
			name:        "filter with only spaces",
			query:       "technology",
			filters:     []string{"   "},
			limit:       10,
			expectError: true,
			description: "Should reject filters with only spaces",
		},
	}

	for _, tt := range securityTests {
		t.Run(tt.name, func(t *testing.T) {
			mockSearchEngine := &MockSearchEngine{
				searchWithFiltersFunc: func(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
					return []domain.SearchDocument{}, nil
				},
			}

			usecase := NewSearchArticlesWithFiltersUsecase(mockSearchEngine)
			_, err := usecase.Execute(ctx, tt.query, tt.filters, tt.limit)

			if tt.expectError && err == nil {
				t.Errorf("expected error for %s, got nil", tt.description)
			} else if !tt.expectError && err != nil {
				t.Errorf("expected no error for %s, got %q", tt.description, err.Error())
			}
		})
	}
}

func TestSearchArticlesWithFiltersUsecase_validateInput(t *testing.T) {
	usecase := &SearchArticlesWithFiltersUsecase{}

	tests := []struct {
		name        string
		query       string
		filters     []string
		limit       int
		expectError bool
	}{
		{
			name:        "valid input",
			query:       "technology",
			filters:     []string{"programming"},
			limit:       10,
			expectError: false,
		},
		{
			name:        "empty query",
			query:       "",
			filters:     []string{"programming"},
			limit:       10,
			expectError: true,
		},
		{
			name:        "query too long",
			query:       string(make([]byte, 1001)),
			filters:     []string{"programming"},
			limit:       10,
			expectError: true,
		},
		{
			name:        "negative limit",
			query:       "technology",
			filters:     []string{"programming"},
			limit:       -1,
			expectError: true,
		},
		{
			name:        "limit too large",
			query:       "technology",
			filters:     []string{"programming"},
			limit:       101,
			expectError: true,
		},
		{
			name:        "too many filters",
			query:       "technology",
			filters:     make([]string, 11),
			limit:       10,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := usecase.validateInput(tt.query, tt.filters, tt.limit)
			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			} else if !tt.expectError && err != nil {
				t.Errorf("expected no error, got %q", err.Error())
			}
		})
	}
}

func BenchmarkSearchArticlesWithFiltersUsecase_Execute(b *testing.B) {
	ctx := context.Background()
	mockSearchEngine := &MockSearchEngine{
		searchWithFiltersFunc: func(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
			return []domain.SearchDocument{
				{
					ID:      "1",
					Title:   "Test Article",
					Content: "Test content",
					Tags:    []string{"test"},
				},
			}, nil
		},
	}

	usecase := NewSearchArticlesWithFiltersUsecase(mockSearchEngine)
	query := "technology"
	filters := []string{"programming", "web-development"}
	limit := 10

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		usecase.Execute(ctx, query, filters, limit)
	}
}
