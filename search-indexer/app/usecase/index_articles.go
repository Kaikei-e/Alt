package usecase

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"search-indexer/domain"
	"search-indexer/port"
	"search-indexer/tokenize"
	"slices"
	"sync"
	"time"

	"github.com/ikawaha/kagome/v2/tokenizer"
)

type IndexPhase int

const (
	PhaseBackfill    IndexPhase = iota // Phase 1: Backfill (past direction)
	PhaseIncremental                   // Phase 2: Incremental (future direction)
)

type IndexArticlesUsecase struct {
	articleRepo  port.ArticleRepository
	searchEngine port.SearchEngine
	tokenizer    *tokenizer.Tokenizer

	// synonymsMu guards synonyms and synonymsDirty. synonyms is the
	// process-wide union of every synonym map registered so far. Meilisearch's
	// synonyms PUT is a full replace, not a merge (there is no incremental/
	// patch endpoint: https://www.meilisearch.com/docs/reference/api/settings/update-synonyms),
	// so a flush always sends the accumulated union rather than just the
	// latest batch's map — otherwise it would erase every synonym registered
	// by earlier batches. synonymsDirty decouples "a batch changed the union"
	// from "Meilisearch received a PUT": registerBatchSynonyms only marks
	// dirty, and FlushSynonyms is the sole place that actually calls
	// RegisterSynonyms. A periodic caller (bootstrap.runSynonymsFlushLoop)
	// controls how often that PUT fires — Meilisearch's own task history
	// retains each settingsUpdate task's full payload indefinitely, so PUTting
	// on every batch (the pre-2026-07-22 behavior) filled the task database
	// and locked out all writes (PM-2026-047).
	synonymsMu    sync.Mutex
	synonyms      map[string][]string
	synonymsDirty bool
}

type IndexResult struct {
	IndexedCount    int
	DeletedCount    int
	LastCreatedAt   *time.Time
	LastID          string
	LastDeletedAt   *time.Time
	Phase           IndexPhase
	BackfillDone    bool
	IncrementalMark *time.Time
}

func NewIndexArticlesUsecase(articleRepo port.ArticleRepository, searchEngine port.SearchEngine, tokenizer *tokenizer.Tokenizer) *IndexArticlesUsecase {
	return &IndexArticlesUsecase{
		articleRepo:  articleRepo,
		searchEngine: searchEngine,
		tokenizer:    tokenizer,
	}
}

// ExecuteBackfill executes Phase 1: Backfill (past direction)
func (u *IndexArticlesUsecase) ExecuteBackfill(ctx context.Context, lastCreatedAt *time.Time, lastID string, batchSize int) (*IndexResult, error) {
	articles, newLastCreatedAt, newLastID, err := u.articleRepo.GetArticlesWithTags(ctx, lastCreatedAt, lastID, batchSize)
	if err != nil {
		return nil, err
	}

	if len(articles) == 0 {
		return &IndexResult{
			IndexedCount:  0,
			LastCreatedAt: lastCreatedAt,
			LastID:        lastID,
			Phase:         PhaseBackfill,
			BackfillDone:  true,
		}, nil
	}

	docs := make([]domain.SearchDocument, 0, len(articles))
	for _, article := range articles {
		docs = append(docs, domain.NewSearchDocument(article))
	}

	if err := u.searchEngine.IndexDocuments(ctx, docs); err != nil {
		return nil, err
	}

	u.registerBatchSynonyms(ctx, docs)

	return &IndexResult{
		IndexedCount:  len(docs),
		LastCreatedAt: newLastCreatedAt,
		LastID:        newLastID,
		Phase:         PhaseBackfill,
		BackfillDone:  false,
	}, nil
}

// ExecuteIncremental executes Phase 2: Incremental (future direction) + deletion sync
func (u *IndexArticlesUsecase) ExecuteIncremental(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, lastDeletedAt *time.Time, batchSize int) (*IndexResult, error) {
	result := &IndexResult{
		Phase:           PhaseIncremental,
		IncrementalMark: incrementalMark,
		LastCreatedAt:   lastCreatedAt,
		LastID:          lastID,
		LastDeletedAt:   lastDeletedAt,
	}

	// 1. Index new articles (future direction)
	articles, newLastCreatedAt, newLastID, err := u.articleRepo.GetArticlesWithTagsForward(ctx, incrementalMark, lastCreatedAt, lastID, batchSize)
	if err != nil {
		return nil, err
	}

	if len(articles) > 0 {
		docs := make([]domain.SearchDocument, 0, len(articles))
		for _, article := range articles {
			docs = append(docs, domain.NewSearchDocument(article))
		}

		if err := u.searchEngine.IndexDocuments(ctx, docs); err != nil {
			return nil, err
		}

		u.registerBatchSynonyms(ctx, docs)

		result.IndexedCount = len(docs)
		result.LastCreatedAt = newLastCreatedAt
		result.LastID = newLastID
	}

	// 2. Sync deletions
	deletedIDs, newLastDeletedAt, err := u.articleRepo.GetDeletedArticles(ctx, lastDeletedAt, batchSize)
	if err != nil {
		return nil, err
	}

	if len(deletedIDs) > 0 {
		if err := u.searchEngine.DeleteDocuments(ctx, deletedIDs); err != nil {
			return nil, err
		}
		result.DeletedCount = len(deletedIDs)
		result.LastDeletedAt = newLastDeletedAt
	}

	return result, nil
}

// GetIncrementalMark gets the latest created_at to use as incrementalMark
func (u *IndexArticlesUsecase) GetIncrementalMark(ctx context.Context) (*time.Time, error) {
	return u.articleRepo.GetLatestCreatedAt(ctx)
}

// ExecuteSingleArticle indexes a single article by its ID (for event-driven indexing).
func (u *IndexArticlesUsecase) ExecuteSingleArticle(ctx context.Context, articleID string) (*IndexResult, error) {
	article, err := u.articleRepo.GetArticleByID(ctx, articleID)
	if errors.Is(err, domain.ErrArticleNotFound) {
		return &IndexResult{
			IndexedCount: 0,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	doc := domain.NewSearchDocument(article)
	if err := u.searchEngine.IndexDocuments(ctx, []domain.SearchDocument{doc}); err != nil {
		return nil, err
	}

	u.registerBatchSynonyms(ctx, []domain.SearchDocument{doc})

	return &IndexResult{
		IndexedCount: 1,
	}, nil
}

// IndexDocumentsDirectly indexes pre-built search documents without repository lookup.
// Used for fat events where the event payload contains all necessary data.
func (u *IndexArticlesUsecase) IndexDocumentsDirectly(ctx context.Context, docs []domain.SearchDocument) (*IndexResult, error) {
	if len(docs) == 0 {
		return &IndexResult{IndexedCount: 0}, nil
	}

	if err := u.searchEngine.IndexDocuments(ctx, docs); err != nil {
		return nil, err
	}

	u.registerBatchSynonyms(ctx, docs)

	return &IndexResult{IndexedCount: len(docs)}, nil
}

// ExecuteBatchArticles indexes multiple articles by their IDs in a single batch.
func (u *IndexArticlesUsecase) ExecuteBatchArticles(ctx context.Context, articleIDs []string) (*IndexResult, error) {
	if len(articleIDs) == 0 {
		return &IndexResult{IndexedCount: 0}, nil
	}

	var docs []domain.SearchDocument
	for _, id := range articleIDs {
		article, err := u.articleRepo.GetArticleByID(ctx, id)
		if errors.Is(err, domain.ErrArticleNotFound) {
			// The article was deleted (or the ID was never valid) between
			// the event being published and this batch running. Skip just
			// this ID instead of failing the whole batch — a hard error
			// here would drop every other, already-durable article in the
			// same batch.
			continue
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, domain.NewSearchDocument(article))
	}

	if len(docs) == 0 {
		return &IndexResult{IndexedCount: 0}, nil
	}

	if err := u.searchEngine.IndexDocuments(ctx, docs); err != nil {
		return nil, err
	}

	u.registerBatchSynonyms(ctx, docs)

	return &IndexResult{IndexedCount: len(docs)}, nil
}

// registerBatchSynonyms merges synonyms for every doc in the batch into the
// process-wide union accumulated across all batches and marks it dirty if
// the batch introduced anything new. It never calls Meilisearch itself —
// FlushSynonyms does that — so indexing throughput never determines how
// often Meilisearch receives a settingsUpdate task.
func (u *IndexArticlesUsecase) registerBatchSynonyms(ctx context.Context, docs []domain.SearchDocument) {
	if len(docs) == 0 {
		return
	}
	batch := make(map[string][]string)
	for _, doc := range docs {
		maps.Copy(batch, tokenize.ProcessTagToSynonyms(u.tokenizer, doc.Tags))
	}
	if len(batch) == 0 {
		return
	}

	u.synonymsMu.Lock()
	defer u.synonymsMu.Unlock()
	if u.synonyms == nil {
		u.synonyms = make(map[string][]string, len(batch))
	}
	changed := false
	for k, v := range batch {
		if existing, ok := u.synonyms[k]; !ok || !slices.Equal(existing, v) {
			changed = true
			break
		}
	}
	if !changed {
		slog.DebugContext(ctx, "batch introduces no new or changed synonym entries", "batch_size", len(batch))
		return
	}
	maps.Copy(u.synonyms, batch)
	u.synonymsDirty = true
}

// FlushSynonyms PUTs the accumulated synonyms union to Meilisearch if
// registerBatchSynonyms has marked it dirty since the last flush, and is a
// no-op otherwise. Meilisearch's synonyms setting has no incremental/patch
// update — only a full-replace PUT
// (https://www.meilisearch.com/docs/reference/api/settings/update-synonyms) —
// and retains every settingsUpdate task's full payload in its task history
// indefinitely. PUTting on every indexed batch (the pre-2026-07-22 behavior)
// generated one such task per batch and, as the union grew, per-task payload
// size grew with it, filling the task database and locking out all writes
// (PM-2026-047). The Meilisearch team's own guidance for task-database growth
// is to control how often settings PUTs are issued rather than expect an
// incremental update path (github.com/meilisearch/meilisearch/discussions/567
// via meilisearch/product#567). Call this from a periodic loop
// (bootstrap.runSynonymsFlushLoop) instead of per batch.
func (u *IndexArticlesUsecase) FlushSynonyms(ctx context.Context) error {
	u.synonymsMu.Lock()
	if !u.synonymsDirty {
		u.synonymsMu.Unlock()
		return nil
	}
	union := maps.Clone(u.synonyms)
	u.synonymsDirty = false
	u.synonymsMu.Unlock()

	if err := u.searchEngine.RegisterSynonyms(ctx, union); err != nil {
		// Non-fatal: search still works without synonyms, just with reduced
		// recall for the affected tags. But this used to be silently
		// swallowed entirely, so a persistent Meilisearch problem here was
		// invisible until someone noticed synonym search wasn't working.
		slog.WarnContext(ctx, "failed to register synonyms", "error", err, "synonym_count", len(union))
		return err
	}
	return nil
}
