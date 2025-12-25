package usecase

import (
	"context"
	"fmt"
	"rag-orchestrator/internal/domain"
	"time"

	"github.com/google/uuid"
)

// RetrieveContextInput defines the input parameters for RetrieveContext.
type RetrieveContextInput struct {
	Query               string
	CandidateArticleIDs []string
}

// RetrieveContextOutput defines the output for RetrieveContext.
type RetrieveContextOutput struct {
	Contexts []ContextItem
}

// ContextItem represents a single retrieved chunk with metadata.
type ContextItem struct {
	ChunkText       string
	URL             string
	Title           string
	PublishedAt     string // ISO8601 string
	Score           float32
	DocumentVersion int
	ChunkID         uuid.UUID
}

// RetrieveContextUsecase defines the interface for retrieving context.
type RetrieveContextUsecase interface {
	Execute(ctx context.Context, input RetrieveContextInput) (*RetrieveContextOutput, error)
}

type retrieveContextUsecase struct {
	chunkRepo domain.RagChunkRepository
	docRepo   domain.RagDocumentRepository
	encoder   domain.VectorEncoder
}

// NewRetrieveContextUsecase creates a new RetrieveContextUsecase.
func NewRetrieveContextUsecase(
	chunkRepo domain.RagChunkRepository,
	docRepo domain.RagDocumentRepository,
	encoder domain.VectorEncoder,
) RetrieveContextUsecase {
	return &retrieveContextUsecase{
		chunkRepo: chunkRepo,
		docRepo:   docRepo, // May be needed for fetching doc details if not joined in repo
		encoder:   encoder,
	}
}

func (u *retrieveContextUsecase) Execute(ctx context.Context, input RetrieveContextInput) (*RetrieveContextOutput, error) {
	if input.Query == "" {
		return nil, fmt.Errorf("query is empty")
	}

	// 1. Embed the query
	embeddings, err := u.encoder.Encode(ctx, []string{input.Query})
	if err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}
	if len(embeddings) != 1 {
		return nil, fmt.Errorf("expected 1 embedding, got %d", len(embeddings))
	}
	queryVector := embeddings[0]

	// 2. Search
	// TODO: Make limit configurable? For now hardcode 5-10 implies "relevant context"
	const searchLimit = 5
	results, err := u.chunkRepo.Search(ctx, queryVector, input.CandidateArticleIDs, searchLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to search chunks: %w", err)
	}

	// 3. Resolve Metadata (URL, Title, etc.)
	// Now populated via SearchResult from RagChunkRepository.

	contexts := make([]ContextItem, 0, len(results))

	for _, res := range results {
		contexts = append(contexts, ContextItem{
			ChunkText:       res.Chunk.Content,
			URL:             res.URL,
			Title:           res.Title,
			PublishedAt:     res.Chunk.CreatedAt.Format(time.RFC3339), // Use Chunk creation time as approximation if PublishedAt not available
			Score:           res.Score,
			DocumentVersion: res.DocumentVersion,
			ChunkID:         res.Chunk.ID,
		})
	}

	return &RetrieveContextOutput{Contexts: contexts}, nil
}
