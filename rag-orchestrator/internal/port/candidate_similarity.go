package port

import (
	"context"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
)

// CandidateSimilarityRepository computes pairwise cosine similarities between
// a seed article and a bounded candidate set using rag-db chunk embeddings.
type CandidateSimilarityRepository interface {
	ComputeCandidateSimilarities(
		ctx context.Context,
		seedArticleID string,
		candidateArticleIDs []string,
		rep domain.SimilarityRepresentation,
		userID uuid.UUID,
	) (*domain.CandidateSimilarityResult, error)
}

// ArticleMetadataReader reads current-version article titles and URLs from rag-db.
type ArticleMetadataReader interface {
	GetArticleMetadataByIDs(
		ctx context.Context,
		articleIDs []string,
		userID uuid.UUID,
	) (map[string]domain.RagDocumentMetadata, error)
}
