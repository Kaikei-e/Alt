package usecase

import (
	"context"
	"fmt"
	"search-indexer/domain"
	"testing"
)

// mockRecapSearchEngine implements port.RecapSearchEngine for testing.
type mockRecapSearchEngine struct {
	docs           []domain.RecapDocument
	estimatedTotal int64
	err            error
	lastQuery      string
	lastLimit      int
}

func (m *mockRecapSearchEngine) EnsureRecapIndex(ctx context.Context) error {
	return m.err
}

func (m *mockRecapSearchEngine) IndexRecapDocuments(ctx context.Context, docs []domain.RecapDocument) error {
	return m.err
}

func (m *mockRecapSearchEngine) SearchRecaps(ctx context.Context, query string, limit int) ([]domain.RecapDocument, int64, error) {
	m.lastQuery = query
	m.lastLimit = limit
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.docs, m.estimatedTotal, nil
}

func TestSearchRecapsUsecase_ExecuteByQuery(t *testing.T) {
	sampleDocs := []domain.RecapDocument{
		{
			ID:       "job1__tech",
			JobID:    "job1",
			Genre:    "tech",
			Summary:  "Technology recap summary",
			TopTerms: []string{"ai", "golang"},
			Tags:     []string{"artificial-intelligence"},
		},
	}

	tests := []struct {
		name           string
		query          string
		limit          int
		mockDocs       []domain.RecapDocument
		mockTotal      int64
		mockErr        error
		wantCount      int
		wantTotal      int64
		wantErr        bool
		wantUsedLimit  int
	}{
		{
			name:          "basic free-text search",
			query:         "technology ai",
			limit:         10,
			mockDocs:      sampleDocs,
			mockTotal:     1,
			wantCount:     1,
			wantTotal:     1,
			wantErr:       false,
			wantUsedLimit: 10,
		},
		{
			name:          "default limit when zero",
			query:         "technology",
			limit:         0,
			mockDocs:      sampleDocs,
			mockTotal:     1,
			wantCount:     1,
			wantTotal:     1,
			wantErr:       false,
			wantUsedLimit: 50,
		},
		{
			name:          "default limit when negative",
			query:         "technology",
			limit:         -5,
			mockDocs:      sampleDocs,
			mockTotal:     1,
			wantCount:     1,
			wantTotal:     1,
			wantErr:       false,
			wantUsedLimit: 50,
		},
		{
			name:          "capped limit when exceeds max",
			query:         "technology",
			limit:         500,
			mockDocs:      sampleDocs,
			mockTotal:     1,
			wantCount:     1,
			wantTotal:     1,
			wantErr:       false,
			wantUsedLimit: 200,
		},
		{
			name:      "search engine error",
			query:     "technology",
			limit:     10,
			mockErr:   fmt.Errorf("meilisearch error"),
			wantCount: 0,
			wantErr:   true,
		},
		{
			name:          "empty results",
			query:         "nonexistent",
			limit:         10,
			mockDocs:      []domain.RecapDocument{},
			mockTotal:     0,
			wantCount:     0,
			wantTotal:     0,
			wantErr:       false,
			wantUsedLimit: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &mockRecapSearchEngine{
				docs:           tt.mockDocs,
				estimatedTotal: tt.mockTotal,
				err:            tt.mockErr,
			}

			uc := NewSearchRecapsUsecase(engine)
			result, err := uc.ExecuteByQuery(context.Background(), tt.query, tt.limit)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ExecuteByQuery() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ExecuteByQuery() unexpected error: %v", err)
			}

			if len(result.Hits) != tt.wantCount {
				t.Errorf("ExecuteByQuery() hit count = %d, want %d", len(result.Hits), tt.wantCount)
			}
			if result.EstimatedTotalHits != tt.wantTotal {
				t.Errorf("ExecuteByQuery() estimated total = %d, want %d", result.EstimatedTotalHits, tt.wantTotal)
			}
			if engine.lastLimit != tt.wantUsedLimit {
				t.Errorf("ExecuteByQuery() passed limit = %d, want %d", engine.lastLimit, tt.wantUsedLimit)
			}
			if engine.lastQuery != tt.query {
				t.Errorf("ExecuteByQuery() passed query = %q, want %q", engine.lastQuery, tt.query)
			}
		})
	}
}
