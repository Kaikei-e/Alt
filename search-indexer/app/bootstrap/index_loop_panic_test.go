package bootstrap

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"search-indexer/config"
	"search-indexer/domain"
	"search-indexer/port"
	"search-indexer/usecase"

	"github.com/ikawaha/kagome/v2/tokenizer"
)

// panicArticleRepo returns one article carrying a Japanese tag so that
// registerBatchSynonyms reaches tokenize.ProcessTagToSynonyms with a nil
// *tokenizer.Tokenizer. Calling Wakati through a nil kagome tokenizer
// pointer panics with a nil-pointer dereference -- this is the exact path
// bootstrap.Run used to leave wired when tokenizer init failed silently.
type panicArticleRepo struct{}

func (panicArticleRepo) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	now := time.Now()
	a, err := domain.NewArticle("art-panic-1", "Title", "Content", []string{"日本語"}, now, "user-1")
	if err != nil {
		return nil, nil, "", err
	}
	return []*domain.Article{a}, &now, "art-panic-1", nil
}

func (panicArticleRepo) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	return panicArticleRepo{}.GetArticlesWithTags(ctx, lastCreatedAt, lastID, limit)
}

func (panicArticleRepo) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]string, *time.Time, error) {
	return nil, nil, nil
}

func (panicArticleRepo) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	return nil, nil
}

func (panicArticleRepo) GetArticleByID(ctx context.Context, articleID string) (*domain.Article, error) {
	return nil, domain.ErrArticleNotFound
}

// noopSearchEngine succeeds on every call so IndexDocuments never itself
// fails -- the panic must come purely from the nil-tokenizer synonym path.
type noopSearchEngine struct{}

func (noopSearchEngine) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	return nil
}
func (noopSearchEngine) DeleteDocuments(ctx context.Context, ids []string) error { return nil }
func (noopSearchEngine) SearchByUserIDWithDateFilter(ctx context.Context, query string, userID string, publishedAfter, publishedBefore *time.Time, limit int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (noopSearchEngine) EnsureIndex(ctx context.Context) error { return nil }
func (noopSearchEngine) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (noopSearchEngine) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error) {
	return nil, 0, nil
}
func (noopSearchEngine) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	return nil
}
func (noopSearchEngine) PruneTaskHistory(ctx context.Context, olderThan time.Duration) error {
	return nil
}

var _ port.ArticleRepository = panicArticleRepo{}
var _ port.SearchEngine = noopSearchEngine{}

// TestSafeExecuteBackfill_RecoversNilTokenizerPanicIntoError reproduces the
// HIGH finding: a nil tokenizer injected into the usecase panics on the
// first Japanese tag, and runIndexLoop's un-scoped recover used to just
// return -- permanently halting indexing while health stayed green. This
// asserts the panic is now converted into a plain error close to the
// source, so the caller's ordinary backoff-and-retry loop handles it.
func TestSafeExecuteBackfill_RecoversNilTokenizerPanicIntoError(t *testing.T) {
	buf := captureLogs(t, slog.LevelInfo)

	uc := usecase.NewIndexArticlesUsecase(panicArticleRepo{}, noopSearchEngine{}, (*tokenizer.Tokenizer)(nil))

	result, err := safeExecuteBackfill(context.Background(), uc, nil, "", 10)

	if err == nil {
		t.Fatal("safeExecuteBackfill() error = nil, want a recovered-panic error")
	}
	if result != nil {
		t.Fatalf("safeExecuteBackfill() result = %+v, want nil on panic", result)
	}
	if !strings.Contains(buf.String(), "backfill panic") {
		t.Fatalf("expected a logged panic record, got: %s", buf.String())
	}
}

// TestSafeExecuteIncremental_RecoversNilTokenizerPanicIntoError mirrors the
// backfill case for Phase 2.
func TestSafeExecuteIncremental_RecoversNilTokenizerPanicIntoError(t *testing.T) {
	buf := captureLogs(t, slog.LevelInfo)

	uc := usecase.NewIndexArticlesUsecase(panicArticleRepo{}, noopSearchEngine{}, (*tokenizer.Tokenizer)(nil))
	now := time.Now()

	result, err := safeExecuteIncremental(context.Background(), uc, &now, nil, "", nil, 10)

	if err == nil {
		t.Fatal("safeExecuteIncremental() error = nil, want a recovered-panic error")
	}
	if result != nil {
		t.Fatalf("safeExecuteIncremental() result = %+v, want nil on panic", result)
	}
	if !strings.Contains(buf.String(), "incremental panic") {
		t.Fatalf("expected a logged panic record, got: %s", buf.String())
	}
}

// TestSafeExecuteBackfill_NoPanicPassesThrough guards against the recover
// wrapper swallowing legitimate results/errors from a healthy tokenizer.
func TestSafeExecuteBackfill_NoPanicPassesThrough(t *testing.T) {
	captureLogs(t, slog.LevelInfo)

	uc := usecase.NewIndexArticlesUsecase(emptyArticleRepo{}, noopSearchEngine{}, nil)

	result, err := safeExecuteBackfill(context.Background(), uc, nil, "", 10)
	if err != nil {
		t.Fatalf("safeExecuteBackfill() unexpected error = %v", err)
	}
	if result == nil || !result.BackfillDone {
		t.Fatalf("safeExecuteBackfill() result = %+v, want BackfillDone=true for an empty repo", result)
	}
}

type emptyArticleRepo struct{}

func (emptyArticleRepo) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	return nil, nil, "", nil
}
func (emptyArticleRepo) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	return nil, nil, "", nil
}
func (emptyArticleRepo) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]string, *time.Time, error) {
	return nil, nil, nil
}
func (emptyArticleRepo) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	return nil, nil
}
func (emptyArticleRepo) GetArticleByID(ctx context.Context, articleID string) (*domain.Article, error) {
	return nil, domain.ErrArticleNotFound
}

var _ port.ArticleRepository = emptyArticleRepo{}

type scriptedLoopRepo struct {
	mu                sync.Mutex
	backfillStep      int
	incrementalStep   int
	incrementalCursor string
	cancel            context.CancelFunc
	t1                time.Time
	t2                time.Time
	t3                time.Time
	t4                time.Time
	backfillGood      *domain.Article
	incrementalGood   *domain.Article
}

func (r *scriptedLoopRepo) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backfillStep++
	switch r.backfillStep {
	case 1:
		// Page 1: all rows skipped, cursor advances to "bf-bad-last".
		return []*domain.Article{}, &r.t1, "bf-bad-last", nil
	case 2:
		if lastID != "bf-bad-last" {
			return []*domain.Article{}, nil, "", nil
		}
		return []*domain.Article{r.backfillGood}, &r.t2, "bf-good-1", nil
	default:
		return []*domain.Article{}, nil, "", nil
	}
}

func (r *scriptedLoopRepo) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.incrementalStep++
	switch r.incrementalStep {
	case 1:
		// Incremental page 1: all rows skipped, cursor advances to "inc-bad-last".
		return []*domain.Article{}, &r.t3, "inc-bad-last", nil
	case 2:
		r.incrementalCursor = lastID
		if r.cancel != nil {
			r.cancel()
		}
		if lastID != "inc-bad-last" {
			return []*domain.Article{}, nil, "", nil
		}
		return []*domain.Article{r.incrementalGood}, &r.t4, "inc-good-1", nil
	default:
		return []*domain.Article{}, nil, "", nil
	}
}

func (r *scriptedLoopRepo) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]string, *time.Time, error) {
	return nil, nil, nil
}

func (r *scriptedLoopRepo) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	return &r.t1, nil
}

func (r *scriptedLoopRepo) GetArticleByID(ctx context.Context, articleID string) (*domain.Article, error) {
	return nil, domain.ErrArticleNotFound
}

type recordingLoopEngine struct {
	mu          sync.Mutex
	indexedDocs []domain.SearchDocument
}

func (e *recordingLoopEngine) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.indexedDocs = append(e.indexedDocs, docs...)
	return nil
}
func (e *recordingLoopEngine) DeleteDocuments(ctx context.Context, ids []string) error { return nil }
func (e *recordingLoopEngine) SearchByUserIDWithDateFilter(ctx context.Context, query string, userID string, publishedAfter, publishedBefore *time.Time, limit int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (e *recordingLoopEngine) EnsureIndex(ctx context.Context) error { return nil }
func (e *recordingLoopEngine) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (e *recordingLoopEngine) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error) {
	return nil, 0, nil
}
func (e *recordingLoopEngine) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	return nil
}
func (e *recordingLoopEngine) PruneTaskHistory(ctx context.Context, olderThan time.Duration) error {
	return nil
}

func TestRunIndexLoop_AdvancesPastAllSkippedPages(t *testing.T) {
	captureLogs(t, slog.LevelInfo)

	prevInterval := config.IndexInterval
	config.IndexInterval = time.Millisecond
	t.Cleanup(func() { config.IndexInterval = prevInterval })

	t1 := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(-time.Minute)
	t3 := t1.Add(time.Minute)
	t4 := t1.Add(2 * time.Minute)

	bfGood, err := domain.NewArticle("bf-good-1", "Backfill Good", "Content", nil, t2, "u1")
	if err != nil {
		t.Fatalf("NewArticle: %v", err)
	}
	incGood, err := domain.NewArticle("inc-good-1", "Incremental Good", "Content", nil, t4, "u1")
	if err != nil {
		t.Fatalf("NewArticle: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	repo := &scriptedLoopRepo{
		cancel:          cancel,
		t1:              t1,
		t2:              t2,
		t3:              t3,
		t4:              t4,
		backfillGood:    bfGood,
		incrementalGood: incGood,
	}
	engine := &recordingLoopEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, engine, nil)

	runIndexLoop(ctx, uc)

	engine.mu.Lock()
	defer engine.mu.Unlock()
	if len(engine.indexedDocs) != 2 {
		t.Fatalf("indexedDocs = %+v, want 2 docs (bf-good-1 and inc-good-1)", engine.indexedDocs)
	}
	if repo.incrementalCursor != "inc-bad-last" {
		t.Fatalf("incremental poll 2 cursor = %q, want %q", repo.incrementalCursor, "inc-bad-last")
	}
}
