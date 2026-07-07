package bootstrap

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

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
func (noopSearchEngine) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (noopSearchEngine) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (noopSearchEngine) SearchWithDateFilter(ctx context.Context, query string, publishedAfter, publishedBefore *time.Time, limit int) ([]domain.SearchDocument, error) {
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
