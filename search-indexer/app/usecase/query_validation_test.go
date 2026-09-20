package usecase

import (
	"context"
	"errors"
	"search-indexer/domain"
	"testing"
	"time"
)

// mockSearchEngine is the shared port.SearchEngine test double for this
// package; search_by_user_test.go and search_articles_fuzz_test.go also
// construct it.
type mockSearchEngine struct {
	indexedDocs []domain.SearchDocument
	err         error

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

func (m *mockSearchEngine) SearchByUserIDWithDateFilter(ctx context.Context, query string, userID string, publishedAfter, publishedBefore *time.Time, limit int) ([]domain.SearchDocument, error) {
	m.dateFilterCalls++
	m.gotDateFilterQuery = query
	m.gotDateFilterUserID = userID
	m.gotPublishedAfter = publishedAfter
	m.gotPublishedBefore = publishedBefore
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

func (m *mockSearchEngine) PruneTaskHistory(ctx context.Context, olderThan time.Duration) error {
	return m.err
}

// TestSearchByUserUsecase_Execute_ValidationBoundaries covers the
// query-validation edge cases that aren't already exercised by
// search_by_user_test.go's Execute_RequiresUserID / Execute_Success: a
// rejected empty query, search-engine error passthrough, and a
// legitimately empty result set.
func TestSearchByUserUsecase_Execute_ValidationBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		mockResults []domain.SearchDocument
		mockErr     error
		wantCount   int
		wantErr     bool
	}{
		{
			name:        "empty query",
			query:       "",
			mockResults: nil,
			mockErr:     nil,
			wantCount:   0,
			wantErr:     true,
		},
		{
			name:        "search engine error",
			query:       "test",
			mockResults: nil,
			mockErr:     &domain.SearchEngineError{Op: "SearchByUserID", Err: errors.New("search failed")},
			wantCount:   0,
			wantErr:     true,
		},
		{
			name:        "no results",
			query:       "nonexistent",
			mockResults: []domain.SearchDocument{},
			mockErr:     nil,
			wantCount:   0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchEngine := &mockSearchEngine{
				indexedDocs: tt.mockResults,
				err:         tt.mockErr,
			}

			usecase := NewSearchByUserUsecase(searchEngine)

			result, err := usecase.Execute(context.Background(), tt.query, "user1")

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

			if len(result.Hits) != tt.wantCount {
				t.Errorf("Execute() result count = %v, want %v", len(result.Hits), tt.wantCount)
			}
		})
	}
}

// TestSearchByUserUsecase_ExecuteWithSecurityValidation verifies H-002's
// allowlist-normalization policy: Meilisearch is not a SQL engine, does not
// render HTML, and does not execute shell commands, so SQLi/XSS/cmd strings
// are just ordinary search tokens. Filter values are escaped separately in
// driver/filter.go (escapeMeilisearchValue). Only structurally dangerous
// characters stay blocked.
func TestSearchByUserUsecase_ExecuteWithSecurityValidation(t *testing.T) {
	searchEngine := &mockSearchEngine{}
	usecase := NewSearchByUserUsecase(searchEngine)

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		// Structural denial: control chars and zero-width are real attack
		// vectors because they break downstream parsers and log analyzers.
		{"null byte injection", "test\x00", true},
		{"carriage return", "test\r\n", true},
		{"vertical tab", "test\v", true},
		{"form feed", "test\f", true},
		{"zero width characters", "test​‌‍", true},

		// Length limit stays enforced.
		{"very long query", string(make([]byte, 1001)), true},

		// Ordinary search queries must not be rejected.
		{"normal search", "golang programming", false},
		{"search with numbers", "python 3.11", false},
		{"search with hyphens", "test-driven development", false},
		{"search with spaces", "clean architecture", false},
		{"search with unicode", "プログラミング", false},
		{"executive summary should pass", "executive summary", false},
		{"selected items should pass", "selected items", false},
		{"union jack flag should pass", "union jack flag", false},

		// Formerly-blocked payloads are just strings to Meilisearch. Users
		// legitimately search for HTML, SQL fragments, and code snippets.
		{"html fragment in query allowed", "<script>alert('xss')</script>", false},
		{"sql fragment in query allowed", "SELECT * FROM users", false},
		{"quote and semicolon allowed", "'; DROP TABLE articles; --", false},
		{"backtick in query allowed", "test`whoami`", false},
		{"pipe in query allowed", "test | rm -rf /", false},
		{"url encoded script allowed", "%3Cscript%3E", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := usecase.Execute(context.Background(), tt.query, "user1")

			if tt.wantErr && err == nil {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
