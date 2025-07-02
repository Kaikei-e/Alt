package usecase

import (
	"context"
	"search-indexer/domain"
	"search-indexer/port"
	"search-indexer/tokenize"
	"time"

	"github.com/ikawaha/kagome/v2/tokenizer"
)

type IndexArticlesUsecase struct {
	articleRepo  port.ArticleRepository
	searchEngine port.SearchEngine
	tokenizer    *tokenizer.Tokenizer
}

type IndexResult struct {
	IndexedCount  int
	LastCreatedAt *time.Time
	LastID        string
}

func NewIndexArticlesUsecase(articleRepo port.ArticleRepository, searchEngine port.SearchEngine, tokenizer *tokenizer.Tokenizer) *IndexArticlesUsecase {
	return &IndexArticlesUsecase{
		articleRepo:  articleRepo,
		searchEngine: searchEngine,
		tokenizer:    tokenizer,
	}
}

func (u *IndexArticlesUsecase) Execute(ctx context.Context, lastCreatedAt *time.Time, lastID string, batchSize int) (*IndexResult, error) {
	articles, newLastCreatedAt, newLastID, err := u.articleRepo.GetArticlesWithTags(ctx, lastCreatedAt, lastID, batchSize)
	if err != nil {
		return nil, err
	}

	if len(articles) == 0 {
		return &IndexResult{
			IndexedCount:  0,
			LastCreatedAt: lastCreatedAt,
			LastID:        lastID,
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
		u.searchEngine.RegisterSynonyms(ctx, synonyms)
	}

	return &IndexResult{
		IndexedCount:  len(docs),
		LastCreatedAt: newLastCreatedAt,
		LastID:        newLastID,
	}, nil
}
