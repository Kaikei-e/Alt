package usecase

import (
	"context"
	"search-indexer/domain"
	"search-indexer/port"
	"search-indexer/tokenize"
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

	for _, doc := range docs {
		synonyms := tokenize.ProcessTagToSynonyms(u.tokenizer, doc.Tags)
		if len(synonyms) > 0 {
			_ = u.searchEngine.RegisterSynonyms(ctx, synonyms)
		}
	}

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

		for _, doc := range docs {
			synonyms := tokenize.ProcessTagToSynonyms(u.tokenizer, doc.Tags)
			if len(synonyms) > 0 {
				_ = u.searchEngine.RegisterSynonyms(ctx, synonyms)
			}
		}

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

// ExecuteSingleArticle indexes a single article by its ID (for event-driven indexing)
func (u *IndexArticlesUsecase) ExecuteSingleArticle(ctx context.Context, articleID string) (*IndexResult, error) {
	article, err := u.articleRepo.GetArticleByID(ctx, articleID)
	if err != nil {
		return nil, err
	}

	if article == nil {
		return &IndexResult{
			IndexedCount: 0,
		}, nil
	}

	doc := domain.NewSearchDocument(article)
	if err := u.searchEngine.IndexDocuments(ctx, []domain.SearchDocument{doc}); err != nil {
		return nil, err
	}

	// Process synonyms for tags
	synonyms := tokenize.ProcessTagToSynonyms(u.tokenizer, doc.Tags)
	if len(synonyms) > 0 {
		_ = u.searchEngine.RegisterSynonyms(ctx, synonyms)
	}

	return &IndexResult{
		IndexedCount: 1,
	}, nil
}
