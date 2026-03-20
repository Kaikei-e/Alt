package usecase

import (
	"context"
	"log/slog"

	"rag-orchestrator/internal/domain"
)

type articleScopedStrategy struct {
	docRepo   domain.RagDocumentRepository
	chunkRepo domain.RagChunkRepository
	logger    *slog.Logger
}

// NewArticleScopedStrategy creates a strategy that retrieves all chunks for a specific article.
func NewArticleScopedStrategy(
	docRepo domain.RagDocumentRepository,
	chunkRepo domain.RagChunkRepository,
	logger *slog.Logger,
) RetrievalStrategy {
	return &articleScopedStrategy{
		docRepo:   docRepo,
		chunkRepo: chunkRepo,
		logger:    logger,
	}
}

func (s *articleScopedStrategy) Name() string { return "article_scoped" }

func (s *articleScopedStrategy) Retrieve(ctx context.Context, _ RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error) {
	doc, err := s.docRepo.GetByArticleID(ctx, intent.ArticleID)
	if err != nil {
		return nil, err
	}
	if doc == nil || doc.CurrentVersionID == nil {
		return nil, ErrArticleNotIndexed
	}

	version, err := s.docRepo.GetVersionByID(ctx, *doc.CurrentVersionID)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, ErrArticleNotIndexed
	}

	chunks, err := s.chunkRepo.GetChunksByVersionID(ctx, *doc.CurrentVersionID)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, ErrArticleNotIndexed
	}

	s.logger.Info("article_scoped_retrieval",
		slog.String("article_id", intent.ArticleID),
		slog.Int("chunks", len(chunks)),
		slog.String("version_id", doc.CurrentVersionID.String()))

	contexts := make([]ContextItem, len(chunks))
	for i, chunk := range chunks {
		contexts[i] = ContextItem{
			ChunkID:         chunk.ID,
			ChunkText:       chunk.Content,
			URL:             version.URL,
			Title:           version.Title,
			PublishedAt:     version.CreatedAt.Format("2006-01-02T15:04:05Z"),
			Score:           1.0,
			DocumentVersion: version.VersionNumber,
		}
	}

	return &RetrieveContextOutput{Contexts: contexts}, nil
}

// selectStrategy returns the strategy for the given intent type.
func (u *answerWithRAGUsecase) selectStrategy(intentType IntentType) RetrievalStrategy {
	if s, ok := u.strategies[intentType]; ok {
		return s
	}
	return u.generalStrategy
}

