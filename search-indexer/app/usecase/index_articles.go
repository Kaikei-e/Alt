package usecase

import (
	"context"
	"errors"
	"maps"
	"search-indexer/domain"
	"search-indexer/port"
	"search-indexer/tokenize"
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

	// synonymsMu guards synonyms, the process-wide union of every synonym
	// map registered so far. Meilisearch's synonyms PUT is a full replace,
	// not a merge, so registerBatchSynonyms must always PUT the accumulated
	// union rather than just the current batch's map — otherwise each
	// batch's PUT erases every synonym registered by earlier batches.
	synonymsMu sync.Mutex
	synonyms   map[string][]string
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
// process-wide union accumulated across all batches, and emits at most one
// RegisterSynonyms call carrying that union. Meilisearch's synonyms PUT is a
// full replace, not a merge: PUTing only the current batch's map erases every
// synonym registered by earlier batches, so only the last batch processed
// ever survived. Emitting the accumulated union PUT also keeps the "at most
// one PUT per batch" property that collapses Meilisearch's synonyms-PUT task
// queue pressure against concurrent search reads.
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
	if u.synonyms == nil {
		u.synonyms = make(map[string][]string, len(batch))
	}
	maps.Copy(u.synonyms, batch)
	union := maps.Clone(u.synonyms)
	u.synonymsMu.Unlock()

	_ = u.searchEngine.RegisterSynonyms(ctx, union)
}
