package usecase

import (
	"context"
	"search-indexer/domain"
	"search-indexer/port"
	"testing"
	"time"
)

// Mock implementations for testing
type mockArticleRepo struct {
	articles []*domain.Article
	err      error
}

func (m *mockArticleRepo) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	if m.err != nil {
		return nil, nil, "", m.err
	}

	if len(m.articles) == 0 {
		return []*domain.Article{}, nil, "", nil
	}

	lastArticle := m.articles[len(m.articles)-1]
	createdAt := lastArticle.CreatedAt()
	return m.articles, &createdAt, lastArticle.ID(), nil
}

type mockSearchEngine struct {
	indexedDocs []domain.SearchDocument
	err         error
}

func (m *mockSearchEngine) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	if m.err != nil {
		return m.err
	}
	m.indexedDocs = append(m.indexedDocs, docs...)
	return nil
}

func (m *mockSearchEngine) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.indexedDocs, nil
}

func (m *mockSearchEngine) EnsureIndex(ctx context.Context) error {
	return nil
}

func (m *mockSearchEngine) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	return nil
}

func TestIndexArticlesUsecase_Execute(t *testing.T) {
	now := time.Now()
	article1, _ := domain.NewArticle("1", "Title 1", "Content 1", []string{"tag1"}, now)
	article2, _ := domain.NewArticle("2", "Title 2", "Content 2", []string{"tag2"}, now.Add(time.Minute))

	tests := []struct {
		name         string
		mockArticles []*domain.Article
		repoErr      error
		searchErr    error
		batchSize    int
		wantIndexed  int
		wantErr      bool
	}{
		{
			name:         "successful indexing",
			mockArticles: []*domain.Article{article1, article2},
			repoErr:      nil,
			searchErr:    nil,
			batchSize:    10,
			wantIndexed:  2,
			wantErr:      false,
		},
		{
			name:         "repository error",
			mockArticles: nil,
			repoErr:      &port.RepositoryError{Op: "GetArticlesWithTags", Err: "db error"},
			searchErr:    nil,
			batchSize:    10,
			wantIndexed:  0,
			wantErr:      true,
		},
		{
			name:         "search engine error",
			mockArticles: []*domain.Article{article1},
			repoErr:      nil,
			searchErr:    &port.SearchEngineError{Op: "IndexDocuments", Err: "index error"},
			batchSize:    10,
			wantIndexed:  0,
			wantErr:      true,
		},
		{
			name:         "no articles to index",
			mockArticles: []*domain.Article{},
			repoErr:      nil,
			searchErr:    nil,
			batchSize:    10,
			wantIndexed:  0,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockArticleRepo{
				articles: tt.mockArticles,
				err:      tt.repoErr,
			}

			searchEngine := &mockSearchEngine{
				err: tt.searchErr,
			}

			usecase := NewIndexArticlesUsecase(repo, searchEngine, nil)

			result, err := usecase.Execute(context.Background(), nil, "", tt.batchSize)

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

			if result.IndexedCount != tt.wantIndexed {
				t.Errorf("Execute() indexed count = %v, want %v", result.IndexedCount, tt.wantIndexed)
			}

			if len(searchEngine.indexedDocs) != tt.wantIndexed {
				t.Errorf("Search engine has %d docs, want %d", len(searchEngine.indexedDocs), tt.wantIndexed)
			}
		})
	}
}

func TestIndexArticlesUsecase_ExecuteWithPagination(t *testing.T) {
	t.Skip("Skipping pagination test - mock doesn't implement proper pagination logic")
}
