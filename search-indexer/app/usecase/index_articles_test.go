package usecase

import (
	"context"
	"search-indexer/domain"
	"search-indexer/tokenize"
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

func (m *mockArticleRepo) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
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

func (m *mockArticleRepo) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]string, *time.Time, error) {
	if m.err != nil {
		return nil, nil, m.err
	}

	return []string{}, nil, nil
}

func (m *mockArticleRepo) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	if m.err != nil {
		return nil, m.err
	}

	if len(m.articles) == 0 {
		return nil, nil
	}

	latest := m.articles[0].CreatedAt()
	for _, article := range m.articles {
		if article.CreatedAt().After(latest) {
			latest = article.CreatedAt()
		}
	}

	return &latest, nil
}

func (m *mockArticleRepo) GetArticleByID(ctx context.Context, articleID string) (*domain.Article, error) {
	if m.err != nil {
		return nil, m.err
	}

	for _, article := range m.articles {
		if article.ID() == articleID {
			return article, nil
		}
	}

	return nil, &domain.RepositoryError{Op: "GetArticleByID", Err: "not found"}
}

type mockSearchEngineForIndexing struct {
	indexedDocs       []domain.SearchDocument
	err               error
	synonymsCallCount int
	lastSynonymsArg   map[string][]string
}

func (m *mockSearchEngineForIndexing) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	if m.err != nil {
		return m.err
	}
	m.indexedDocs = append(m.indexedDocs, docs...)
	return nil
}

func (m *mockSearchEngineForIndexing) DeleteDocuments(ctx context.Context, ids []string) error {
	if m.err != nil {
		return m.err
	}
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
	return nil
}

func (m *mockSearchEngineForIndexing) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.indexedDocs, nil
}

func (m *mockSearchEngineForIndexing) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.indexedDocs, nil
}

func (m *mockSearchEngineForIndexing) SearchWithDateFilter(ctx context.Context, query string, publishedAfter, publishedBefore *time.Time, limit int) ([]domain.SearchDocument, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.indexedDocs, nil
}

func (m *mockSearchEngineForIndexing) EnsureIndex(ctx context.Context) error {
	return nil
}

func (m *mockSearchEngineForIndexing) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error) {
	return nil, nil
}

func (m *mockSearchEngineForIndexing) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error) {
	return nil, 0, nil
}

func (m *mockSearchEngineForIndexing) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	m.synonymsCallCount++
	m.lastSynonymsArg = synonyms
	return nil
}

func TestIndexArticlesUsecase_Execute(t *testing.T) {
	now := time.Now()
	article1, _ := domain.NewArticle("1", "Title 1", "Content 1", []string{"tag1"}, now, "user1")
	article2, _ := domain.NewArticle("2", "Title 2", "Content 2", []string{"tag2"}, now.Add(time.Minute), "user2")

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
			repoErr:      &domain.RepositoryError{Op: "GetArticlesWithTags", Err: "db error"},
			searchErr:    nil,
			batchSize:    10,
			wantIndexed:  0,
			wantErr:      true,
		},
		{
			name:         "search engine error",
			mockArticles: []*domain.Article{article1},
			repoErr:      nil,
			searchErr:    &domain.SearchEngineError{Op: "IndexDocuments", Err: "index error"},
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

			searchEngine := &mockSearchEngineForIndexing{
				err: tt.searchErr,
			}

			usecase := NewIndexArticlesUsecase(repo, searchEngine, nil)

			result, err := usecase.ExecuteBackfill(context.Background(), nil, "", tt.batchSize)

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

// TestExecuteBackfill_CoalescesSynonymsToSingleCall validates that a batch of
// articles triggers at most one RegisterSynonyms invocation. Each Meilisearch
// settings PUT serialises against search in the LMDB task queue, so the
// per-doc pattern (one PUT per article) saturated the articles index and
// caused /v1/search idle time to spike to 5s+, exceeding the 3s section
// timeout in the global search usecase and forcing the FE into the degraded
// "articles unavailable" state. The batch must merge all per-doc synonyms
// first and emit a single PUT covering the union.
func TestExecuteBackfill_CoalescesSynonymsToSingleCall(t *testing.T) {
	now := time.Now()
	// Distinct Japanese tags across articles so each contributes at least one
	// synonym entry. The pre-coalescing code calls RegisterSynonyms per doc
	// and ends up with 3 PUTs; the coalesced code should emit exactly 1.
	a1, _ := domain.NewArticle("1", "T1", "C1", []string{"テスト1"}, now, "u")
	a2, _ := domain.NewArticle("2", "T2", "C2", []string{"テスト2"}, now.Add(time.Second), "u")
	a3, _ := domain.NewArticle("3", "T3", "C3", []string{"テスト3"}, now.Add(2*time.Second), "u")

	tok, err := tokenize.InitTokenizer()
	if err != nil {
		t.Fatalf("InitTokenizer: %v", err)
	}

	repo := &mockArticleRepo{articles: []*domain.Article{a1, a2, a3}}
	engine := &mockSearchEngineForIndexing{}

	u := NewIndexArticlesUsecase(repo, engine, tok)
	if _, err := u.ExecuteBackfill(context.Background(), nil, "", 10); err != nil {
		t.Fatalf("ExecuteBackfill: %v", err)
	}

	if engine.synonymsCallCount != 1 {
		t.Fatalf("RegisterSynonyms call count = %d, want 1 (coalesced per batch)", engine.synonymsCallCount)
	}

	// The single call must carry the union of all docs' synonyms.
	for _, want := range []string{"テスト1", "テスト2", "テスト3"} {
		if _, ok := engine.lastSynonymsArg[want]; !ok {
			t.Errorf("coalesced synonyms missing key %q; got map=%v", want, engine.lastSynonymsArg)
		}
	}
}

// TestIndexDocumentsDirectly_CoalescesSynonymsToSingleCall pins the same
// invariant for the fat-event path used in production (ArticleCreated
// events land here via the Connect-RPC consumer). This is the code path
// whose 15-second synonyms PUT cadence blocked /indexes/articles/search.
func TestIndexDocumentsDirectly_CoalescesSynonymsToSingleCall(t *testing.T) {
	now := time.Now()
	tok, err := tokenize.InitTokenizer()
	if err != nil {
		t.Fatalf("InitTokenizer: %v", err)
	}

	build := func(id, tag string) domain.SearchDocument {
		a, _ := domain.NewArticle(id, "t", "c", []string{tag}, now, "u")
		return domain.NewSearchDocument(a)
	}
	docs := []domain.SearchDocument{
		build("1", "日本語タグA"),
		build("2", "日本語タグB"),
		build("3", "日本語タグC"),
	}

	engine := &mockSearchEngineForIndexing{}
	u := NewIndexArticlesUsecase(&mockArticleRepo{}, engine, tok)

	if _, err := u.IndexDocumentsDirectly(context.Background(), docs); err != nil {
		t.Fatalf("IndexDocumentsDirectly: %v", err)
	}

	if engine.synonymsCallCount != 1 {
		t.Fatalf("RegisterSynonyms call count = %d, want 1", engine.synonymsCallCount)
	}
	for _, want := range []string{"日本語タグA", "日本語タグB", "日本語タグC"} {
		if _, ok := engine.lastSynonymsArg[want]; !ok {
			t.Errorf("coalesced synonyms missing key %q", want)
		}
	}
}

// TestExecuteBackfill_SkipsSynonymsWhenAllTagsNonJapanese ensures the batch
// path does not emit an empty PUT. The previous per-doc code avoided this
// via the `len(synonyms) > 0` guard; the coalesced implementation must
// preserve that invariant, otherwise we would spam Meilisearch with
// no-op PUTs that still serialise against search.
func TestExecuteBackfill_SkipsSynonymsWhenAllTagsNonJapanese(t *testing.T) {
	now := time.Now()
	a1, _ := domain.NewArticle("1", "T1", "C1", []string{"english-only"}, now, "u")
	a2, _ := domain.NewArticle("2", "T2", "C2", []string{"another-english"}, now.Add(time.Second), "u")

	tok, err := tokenize.InitTokenizer()
	if err != nil {
		t.Fatalf("InitTokenizer: %v", err)
	}

	engine := &mockSearchEngineForIndexing{}
	u := NewIndexArticlesUsecase(&mockArticleRepo{articles: []*domain.Article{a1, a2}}, engine, tok)

	if _, err := u.ExecuteBackfill(context.Background(), nil, "", 10); err != nil {
		t.Fatalf("ExecuteBackfill: %v", err)
	}

	if engine.synonymsCallCount != 0 {
		t.Fatalf("RegisterSynonyms call count = %d, want 0 (no Japanese tags)", engine.synonymsCallCount)
	}
}
