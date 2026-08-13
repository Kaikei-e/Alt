package usecase

import (
	"context"
	"errors"
	"fmt"
	"search-indexer/domain"
	"search-indexer/tokenize"
	"slices"
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

	return nil, domain.ErrArticleNotFound
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

func (m *mockSearchEngineForIndexing) PruneTaskHistory(ctx context.Context, olderThan time.Duration) error {
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
			repoErr:      &domain.RepositoryError{Op: "GetArticlesWithTags", Err: errors.New("db error")},
			searchErr:    nil,
			batchSize:    10,
			wantIndexed:  0,
			wantErr:      true,
		},
		{
			name:         "search engine error",
			mockArticles: []*domain.Article{article1},
			repoErr:      nil,
			searchErr:    &domain.SearchEngineError{Op: "IndexDocuments", Err: errors.New("index error")},
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

// mockPaginatingArticleRepo simulates the driver-layer cursor pagination
// contract: GetArticlesWithTags(lastCreatedAt, lastID, limit) returns the
// page strictly after the given cursor, not the whole dataset regardless of
// cursor. mockArticleRepo above always returns its full articles slice no
// matter what cursor is passed, so it cannot exercise the pagination loop --
// which is exactly why this test used to be skipped ("mock doesn't
// implement proper pagination logic"). This mock tracks position by article
// ID instead, mirroring how the real SQL cursor advances.
type mockPaginatingArticleRepo struct {
	all []*domain.Article
}

func (m *mockPaginatingArticleRepo) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	start := 0
	if lastID != "" {
		for i, a := range m.all {
			if a.ID() == lastID {
				start = i + 1
				break
			}
		}
	}
	if start >= len(m.all) {
		return []*domain.Article{}, lastCreatedAt, lastID, nil
	}
	end := start + limit
	if end > len(m.all) {
		end = len(m.all)
	}
	page := m.all[start:end]
	last := page[len(page)-1]
	createdAt := last.CreatedAt()
	return page, &createdAt, last.ID(), nil
}

func (m *mockPaginatingArticleRepo) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	return nil, nil, "", errors.New("mockPaginatingArticleRepo: GetArticlesWithTagsForward not used by this test")
}

func (m *mockPaginatingArticleRepo) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]string, *time.Time, error) {
	return nil, nil, nil
}

func (m *mockPaginatingArticleRepo) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	return nil, nil
}

func (m *mockPaginatingArticleRepo) GetArticleByID(ctx context.Context, articleID string) (*domain.Article, error) {
	return nil, domain.ErrArticleNotFound
}

// TestIndexArticlesUsecase_ExecuteWithPagination drives ExecuteBackfill
// across multiple pages the way bootstrap's backfill loop does in
// production: repeatedly call it with the previous call's LastCreatedAt/
// LastID cursor until BackfillDone is true. It asserts every article is
// indexed exactly once (no page skipped, no page re-indexed) and that the
// loop terminates. This was previously an empty, skipped test -- the
// pagination cursor hand-off had no coverage at all.
func TestIndexArticlesUsecase_ExecuteWithPagination(t *testing.T) {
	now := time.Now()
	const articleCount = 5
	articles := make([]*domain.Article, 0, articleCount)
	for i := range articleCount {
		a, err := domain.NewArticle(fmt.Sprintf("art-%d", i), fmt.Sprintf("Title %d", i), "Content", nil, now.Add(time.Duration(i)*time.Minute), "user")
		if err != nil {
			t.Fatalf("NewArticle(%d): %v", i, err)
		}
		articles = append(articles, a)
	}

	repo := &mockPaginatingArticleRepo{all: articles}
	engine := &mockSearchEngineForIndexing{}
	u := NewIndexArticlesUsecase(repo, engine, nil)

	const pageSize = 2
	var lastCreatedAt *time.Time
	lastID := ""
	totalIndexed := 0

	for page := 1; ; page++ {
		if page > articleCount+2 {
			t.Fatal("pagination loop did not terminate (BackfillDone never became true)")
		}
		result, err := u.ExecuteBackfill(context.Background(), lastCreatedAt, lastID, pageSize)
		if err != nil {
			t.Fatalf("ExecuteBackfill (page %d): %v", page, err)
		}
		if result.BackfillDone {
			if result.IndexedCount != 0 {
				t.Errorf("final page: IndexedCount = %d, want 0", result.IndexedCount)
			}
			break
		}
		if result.IndexedCount == 0 {
			t.Fatalf("page %d: IndexedCount = 0 but BackfillDone = false; loop would spin forever", page)
		}
		totalIndexed += result.IndexedCount
		lastCreatedAt = result.LastCreatedAt
		lastID = result.LastID
	}

	if totalIndexed != articleCount {
		t.Fatalf("total indexed across pages = %d, want %d", totalIndexed, articleCount)
	}
	if len(engine.indexedDocs) != articleCount {
		t.Fatalf("search engine has %d docs, want %d (a page was skipped or double-indexed)", len(engine.indexedDocs), articleCount)
	}

	seen := make(map[string]int, articleCount)
	for _, doc := range engine.indexedDocs {
		seen[doc.ID]++
	}
	for _, a := range articles {
		if seen[a.ID()] != 1 {
			t.Errorf("article %s indexed %d times, want exactly 1", a.ID(), seen[a.ID()])
		}
	}
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
	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms: %v", err)
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
	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms: %v", err)
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
	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms: %v", err)
	}

	if engine.synonymsCallCount != 0 {
		t.Fatalf("RegisterSynonyms call count = %d, want 0 (no Japanese tags)", engine.synonymsCallCount)
	}
}

// TestExecuteBatchArticles_SkipsNotFoundArticle reproduces the HIGH finding
// where a single deleted article ID in a batch failed the whole call,
// dropping every other (already-ACKed) article alongside it. The gateway
// now signals not-found via domain.ErrArticleNotFound instead of an opaque
// *domain.RepositoryError, so the usecase can skip just that ID.
func TestExecuteBatchArticles_SkipsNotFoundArticle(t *testing.T) {
	now := time.Now()
	article1, _ := domain.NewArticle("keep-1", "T1", "C1", []string{}, now, "u")
	repo := &mockArticleRepo{articles: []*domain.Article{article1}}
	engine := &mockSearchEngineForIndexing{}

	u := NewIndexArticlesUsecase(repo, engine, nil)

	result, err := u.ExecuteBatchArticles(context.Background(), []string{"keep-1", "deleted-id"})
	if err != nil {
		t.Fatalf("ExecuteBatchArticles() error = %v, want nil (not-found articles must be skipped)", err)
	}
	if result.IndexedCount != 1 {
		t.Fatalf("IndexedCount = %d, want 1", result.IndexedCount)
	}
	if len(engine.indexedDocs) != 1 || engine.indexedDocs[0].ID != "keep-1" {
		t.Fatalf("indexedDocs = %+v, want exactly [keep-1]", engine.indexedDocs)
	}
}

// TestExecuteBatchArticles_PropagatesNonNotFoundError ensures a genuine
// repository failure still fails the batch instead of being swallowed
// alongside the not-found skip path.
func TestExecuteBatchArticles_PropagatesNonNotFoundError(t *testing.T) {
	repo := &mockArticleRepo{err: &domain.RepositoryError{Op: "GetArticleByID", Err: errors.New("db unavailable")}}
	engine := &mockSearchEngineForIndexing{}
	u := NewIndexArticlesUsecase(repo, engine, nil)

	if _, err := u.ExecuteBatchArticles(context.Background(), []string{"any-id"}); err == nil {
		t.Fatal("ExecuteBatchArticles() error = nil, want error for a non-not-found repository failure")
	}
}

// TestExecuteSingleArticle_NotFound preserves the "0 indexed, no error"
// contract for a single not-found article now that not-found is signalled
// via an error rather than a nil article + nil error pair.
func TestExecuteSingleArticle_NotFound(t *testing.T) {
	repo := &mockArticleRepo{}
	engine := &mockSearchEngineForIndexing{}
	u := NewIndexArticlesUsecase(repo, engine, nil)

	result, err := u.ExecuteSingleArticle(context.Background(), "missing")
	if err != nil {
		t.Fatalf("ExecuteSingleArticle() error = %v, want nil", err)
	}
	if result.IndexedCount != 0 {
		t.Fatalf("IndexedCount = %d, want 0", result.IndexedCount)
	}
}

// TestRegisterBatchSynonyms_AccumulatesAcrossBatches reproduces the HIGH
// finding where Meilisearch's synonyms PUT is a full replace: calling
// RegisterSynonyms with only the current batch's map erased every synonym
// registered by an earlier batch, so only the last batch's synonyms
// survived. The usecase must accumulate a process-wide union and PUT that
// union on every flush.
func TestRegisterBatchSynonyms_AccumulatesAcrossBatches(t *testing.T) {
	now := time.Now()
	tok, err := tokenize.InitTokenizer()
	if err != nil {
		t.Fatalf("InitTokenizer: %v", err)
	}

	engine := &mockSearchEngineForIndexing{}

	a1, _ := domain.NewArticle("1", "T1", "C1", []string{"テスト1"}, now, "u")
	repo1 := &mockArticleRepo{articles: []*domain.Article{a1}}
	u := NewIndexArticlesUsecase(repo1, engine, tok)
	if _, err := u.ExecuteBackfill(context.Background(), nil, "", 10); err != nil {
		t.Fatalf("ExecuteBackfill (batch 1): %v", err)
	}
	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms (batch 1): %v", err)
	}
	if _, ok := engine.lastSynonymsArg["テスト1"]; !ok {
		t.Fatalf("batch 1 synonyms missing テスト1: %v", engine.lastSynonymsArg)
	}

	// Second batch on the SAME usecase instance — mirrors production, where
	// one long-lived IndexArticlesUsecase is shared across the polling loop
	// and the event-driven consumer flush — with a different Japanese tag.
	a2, _ := domain.NewArticle("2", "T2", "C2", []string{"テスト2"}, now.Add(time.Second), "u")
	u.articleRepo = &mockArticleRepo{articles: []*domain.Article{a2}}
	if _, err := u.ExecuteBackfill(context.Background(), nil, "", 10); err != nil {
		t.Fatalf("ExecuteBackfill (batch 2): %v", err)
	}
	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms (batch 2): %v", err)
	}

	if _, ok := engine.lastSynonymsArg["テスト2"]; !ok {
		t.Fatalf("batch 2 synonyms missing テスト2: %v", engine.lastSynonymsArg)
	}
	if _, ok := engine.lastSynonymsArg["テスト1"]; !ok {
		t.Fatalf("batch 2 PUT dropped テスト1 from an earlier batch (full-replace overwrite bug): %v", engine.lastSynonymsArg)
	}
}

// TestRegisterBatchSynonyms_SkipsPUTWhenNoNewSynonyms reproduces PM-2026-047:
// registerBatchSynonyms PUT the accumulated union unconditionally on every
// flush, even when the batch contributed no synonym not already present in
// the process-wide union. In production, repeated tags across a continuous
// stream of articles meant nearly every indexing operation re-sent the full
// dictionary, generating one Meilisearch settingsUpdate task per flush
// (976,598 accumulated tasks, ~15.7GB) until the task database's LMDB map
// filled and Meilisearch rejected all writes, including its own task
// deletions. The usecase must skip the RegisterSynonyms call when the batch
// contains no new key and no changed value versus what was already PUT.
func TestRegisterBatchSynonyms_SkipsPUTWhenNoNewSynonyms(t *testing.T) {
	now := time.Now()
	tok, err := tokenize.InitTokenizer()
	if err != nil {
		t.Fatalf("InitTokenizer: %v", err)
	}

	engine := &mockSearchEngineForIndexing{}

	a1, _ := domain.NewArticle("1", "T1", "C1", []string{"テスト1"}, now, "u")
	repo1 := &mockArticleRepo{articles: []*domain.Article{a1}}
	u := NewIndexArticlesUsecase(repo1, engine, tok)
	if _, err := u.ExecuteBackfill(context.Background(), nil, "", 10); err != nil {
		t.Fatalf("ExecuteBackfill (batch 1): %v", err)
	}
	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms (batch 1): %v", err)
	}
	if engine.synonymsCallCount != 1 {
		t.Fatalf("after batch 1, RegisterSynonyms call count = %d, want 1", engine.synonymsCallCount)
	}

	// Second batch carries the SAME Japanese tag as batch 1, so
	// ProcessTagToSynonyms produces the identical {tag: tokens} entry
	// already present in the accumulated union — nothing new to PUT.
	a2, _ := domain.NewArticle("2", "T2", "C2", []string{"テスト1"}, now.Add(time.Second), "u")
	u.articleRepo = &mockArticleRepo{articles: []*domain.Article{a2}}
	if _, err := u.ExecuteBackfill(context.Background(), nil, "", 10); err != nil {
		t.Fatalf("ExecuteBackfill (batch 2): %v", err)
	}
	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms (batch 2): %v", err)
	}

	if engine.synonymsCallCount != 1 {
		t.Fatalf("after batch 2 with no new synonyms, RegisterSynonyms call count = %d, want 1 (PUT must be skipped)", engine.synonymsCallCount)
	}

	// A third batch that DOES introduce a new tag must still PUT — the skip
	// guard must not become permanently stuck once tripped.
	a3, _ := domain.NewArticle("3", "T3", "C3", []string{"テスト2"}, now.Add(2*time.Second), "u")
	u.articleRepo = &mockArticleRepo{articles: []*domain.Article{a3}}
	if _, err := u.ExecuteBackfill(context.Background(), nil, "", 10); err != nil {
		t.Fatalf("ExecuteBackfill (batch 3): %v", err)
	}
	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms (batch 3): %v", err)
	}
	if engine.synonymsCallCount != 2 {
		t.Fatalf("after batch 3 with a genuinely new synonym, RegisterSynonyms call count = %d, want 2", engine.synonymsCallCount)
	}
	if _, ok := engine.lastSynonymsArg["テスト1"]; !ok {
		t.Fatalf("batch 3 PUT dropped テスト1 from an earlier batch: %v", engine.lastSynonymsArg)
	}
	if _, ok := engine.lastSynonymsArg["テスト2"]; !ok {
		t.Fatalf("batch 3 PUT missing new key テスト2: %v", engine.lastSynonymsArg)
	}
}

// TestFlushSynonyms_NoOpWhenNothingPending reproduces PM-2026-047 action item
// #2: Meilisearch's synonyms setting has no incremental/patch update, only a
// full-replace PUT (https://www.meilisearch.com/docs/reference/api/settings/update-synonyms),
// and the Meilisearch team's own guidance for task-database growth is to
// control how often settings PUTs are issued (see
// github.com/meilisearch/meilisearch/discussions/567). registerBatchSynonyms
// must only mark the union dirty; FlushSynonyms is the sole place that emits
// a RegisterSynonyms call, so indexing and PUT frequency are decoupled and a
// periodic loop (not every batch) controls how often Meilisearch receives a
// settingsUpdate task.
func TestFlushSynonyms_NoOpWhenNothingPending(t *testing.T) {
	engine := &mockSearchEngineForIndexing{}
	u := NewIndexArticlesUsecase(&mockArticleRepo{}, engine, nil)

	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms: %v", err)
	}
	if engine.synonymsCallCount != 0 {
		t.Fatalf("RegisterSynonyms call count = %d, want 0 (nothing pending)", engine.synonymsCallCount)
	}
}

// TestFlushSynonyms_IndexingAloneDoesNotPUT pins the decoupling: indexing a
// batch must only accumulate the union in memory. Without an explicit
// FlushSynonyms call, no PUT is emitted no matter how many batches run.
func TestFlushSynonyms_IndexingAloneDoesNotPUT(t *testing.T) {
	now := time.Now()
	tok, err := tokenize.InitTokenizer()
	if err != nil {
		t.Fatalf("InitTokenizer: %v", err)
	}
	a1, _ := domain.NewArticle("1", "T1", "C1", []string{"テスト1"}, now, "u")
	repo := &mockArticleRepo{articles: []*domain.Article{a1}}
	engine := &mockSearchEngineForIndexing{}
	u := NewIndexArticlesUsecase(repo, engine, tok)

	if _, err := u.ExecuteBackfill(context.Background(), nil, "", 10); err != nil {
		t.Fatalf("ExecuteBackfill: %v", err)
	}
	if engine.synonymsCallCount != 0 {
		t.Fatalf("RegisterSynonyms call count = %d before any FlushSynonyms call, want 0", engine.synonymsCallCount)
	}

	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms (1st): %v", err)
	}
	if engine.synonymsCallCount != 1 {
		t.Fatalf("RegisterSynonyms call count = %d after 1st flush, want 1", engine.synonymsCallCount)
	}

	// A second flush with nothing new accumulated since must stay a no-op —
	// this is what keeps a periodic flush loop from re-PUTting an unchanged
	// dictionary every tick.
	if err := u.FlushSynonyms(context.Background()); err != nil {
		t.Fatalf("FlushSynonyms (2nd): %v", err)
	}
	if engine.synonymsCallCount != 1 {
		t.Fatalf("RegisterSynonyms call count = %d after 2nd flush with nothing new, want 1", engine.synonymsCallCount)
	}
}

// mockPerIDArticleRepo fails GetArticleByID for selected IDs only, which is
// what a transient backend problem looks like from the batch's point of view:
// most IDs resolve, a few return CodeInternal / CodeUnavailable while a
// connection pool is exhausted or the database blips.
type mockPerIDArticleRepo struct {
	articles map[string]*domain.Article
	errByID  map[string]error
}

func (m *mockPerIDArticleRepo) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	return nil, nil, "", errors.New("mockPerIDArticleRepo: GetArticlesWithTags not used by this test")
}

func (m *mockPerIDArticleRepo) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	return nil, nil, "", errors.New("mockPerIDArticleRepo: GetArticlesWithTagsForward not used by this test")
}

func (m *mockPerIDArticleRepo) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]string, *time.Time, error) {
	return nil, nil, nil
}

func (m *mockPerIDArticleRepo) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	return nil, nil
}

func (m *mockPerIDArticleRepo) GetArticleByID(ctx context.Context, articleID string) (*domain.Article, error) {
	if err, ok := m.errByID[articleID]; ok {
		return nil, err
	}
	if a, ok := m.articles[articleID]; ok {
		return a, nil
	}
	return nil, domain.ErrArticleNotFound
}

// batchOfArticles builds n articles named art-0..art-(n-1).
func batchOfArticles(t *testing.T, n int) (map[string]*domain.Article, []string) {
	t.Helper()
	now := time.Now()
	articles := make(map[string]*domain.Article, n)
	ids := make([]string, 0, n)
	for i := range n {
		id := fmt.Sprintf("art-%d", i)
		a, err := domain.NewArticle(id, "Title", "Content", nil, now.Add(time.Duration(i)*time.Second), "user")
		if err != nil {
			t.Fatalf("NewArticle(%s): %v", id, err)
		}
		articles[id] = a
		ids = append(ids, id)
	}
	return articles, ids
}

// TestExecuteBatchArticles_PartialFailureIndexesHealthyArticles covers the
// second half of the batch-indexing HIGH finding. Skipping deleted articles
// fixed the not-found case, but any other per-article error still aborted the
// whole batch: 10 events in, 1 transient backend failure, 0 articles indexed
// and 0 messages ACKed, so all 10 burned a delivery attempt and eventually
// landed in a DLQ nothing reads. Now that alt-data-hub correctly reports
// pool exhaustion / database blips as CodeInternal / CodeUnavailable instead
// of collapsing them into CodeNotFound, this is the common failure shape, so
// the batch must index and report the 9 healthy articles and isolate the
// failure to the one ID that actually failed.
func TestExecuteBatchArticles_PartialFailureIndexesHealthyArticles(t *testing.T) {
	articles, ids := batchOfArticles(t, 10)
	transient := &domain.RepositoryError{Op: "GetArticleByID", Err: errors.New("connection pool exhausted")}
	delete(articles, "art-4")
	repo := &mockPerIDArticleRepo{articles: articles, errByID: map[string]error{"art-4": transient}}
	engine := &mockSearchEngineForIndexing{}
	u := NewIndexArticlesUsecase(repo, engine, nil)

	result, err := u.ExecuteBatchArticles(context.Background(), ids)

	if err == nil {
		t.Fatal("ExecuteBatchArticles() error = nil, want non-nil: the caller must be able to see that one article failed")
	}
	if !errors.Is(err, transient) {
		t.Errorf("ExecuteBatchArticles() error = %v, want it to wrap the underlying per-article failure", err)
	}
	if result == nil {
		t.Fatal("ExecuteBatchArticles() result = nil on partial failure; the caller cannot ACK the 9 durable articles without it")
	}
	if result.IndexedCount != 9 {
		t.Errorf("IndexedCount = %d, want 9 (the healthy articles must not be dropped with the failing one)", result.IndexedCount)
	}
	if len(engine.indexedDocs) != 9 {
		t.Errorf("indexed docs = %d, want 9", len(engine.indexedDocs))
	}
	if !slices.Equal(result.FailedIDs, []string{"art-4"}) {
		t.Errorf("FailedIDs = %v, want [art-4]", result.FailedIDs)
	}
	if len(result.IndexedIDs) != 9 || slices.Contains(result.IndexedIDs, "art-4") {
		t.Errorf("IndexedIDs = %v, want the 9 healthy IDs without art-4", result.IndexedIDs)
	}
	if len(result.SkippedIDs) != 0 {
		t.Errorf("SkippedIDs = %v, want empty (a transient failure is not a skip)", result.SkippedIDs)
	}
}

// TestExecuteBatchArticles_ReportsSkippedSeparatelyFromFailed keeps the two
// terminal-vs-retryable outcomes distinguishable for the caller: a deleted
// article is safe to ACK forever, a transient failure must be retried.
func TestExecuteBatchArticles_ReportsSkippedSeparatelyFromFailed(t *testing.T) {
	articles, _ := batchOfArticles(t, 3)
	transient := errors.New("upstream unavailable")
	repo := &mockPerIDArticleRepo{
		articles: articles,
		errByID:  map[string]error{"art-2": transient},
	}
	engine := &mockSearchEngineForIndexing{}
	u := NewIndexArticlesUsecase(repo, engine, nil)

	result, err := u.ExecuteBatchArticles(context.Background(), []string{"art-0", "deleted-id", "art-2"})
	if err == nil {
		t.Fatal("ExecuteBatchArticles() error = nil, want non-nil for the transient failure")
	}
	if result == nil {
		t.Fatal("ExecuteBatchArticles() result = nil on partial failure; the caller cannot tell a terminal skip from a retryable failure")
	}
	if !slices.Equal(result.IndexedIDs, []string{"art-0"}) {
		t.Errorf("IndexedIDs = %v, want [art-0]", result.IndexedIDs)
	}
	if !slices.Equal(result.SkippedIDs, []string{"deleted-id"}) {
		t.Errorf("SkippedIDs = %v, want [deleted-id]", result.SkippedIDs)
	}
	if !slices.Equal(result.FailedIDs, []string{"art-2"}) {
		t.Errorf("FailedIDs = %v, want [art-2]", result.FailedIDs)
	}
}

// TestExecuteBatchArticles_IndexWriteFailureFailsEveryFetchedID pins the
// other direction: IndexDocuments is one Meilisearch write covering the whole
// batch, so when it fails nothing in it is durable and every fetched ID must
// be reported as failed (never as indexed).
func TestExecuteBatchArticles_IndexWriteFailureFailsEveryFetchedID(t *testing.T) {
	articles, ids := batchOfArticles(t, 3)
	repo := &mockPerIDArticleRepo{articles: articles}
	engine := &mockSearchEngineForIndexing{err: errors.New("meilisearch unreachable")}
	u := NewIndexArticlesUsecase(repo, engine, nil)

	result, err := u.ExecuteBatchArticles(context.Background(), ids)
	if err == nil {
		t.Fatal("ExecuteBatchArticles() error = nil, want non-nil when the index write fails")
	}
	if result == nil {
		t.Fatal("ExecuteBatchArticles() result = nil; the caller cannot tell which IDs to retry")
	}
	if result.IndexedCount != 0 || len(result.IndexedIDs) != 0 {
		t.Errorf("IndexedCount = %d, IndexedIDs = %v, want 0 / empty when nothing was durably written", result.IndexedCount, result.IndexedIDs)
	}
	if !slices.Equal(result.FailedIDs, ids) {
		t.Errorf("FailedIDs = %v, want all fetched IDs %v", result.FailedIDs, ids)
	}
}
