package usecase

import (
	"context"
	"fmt"
	"rag-orchestrator/internal/domain"

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
	// The Search result currently returns Chunk and Score.
	// We need Document info (URL, Title, PublishedAt).
	// Currently Search joins RagDocuments but only returns RagChunk.
	// Option A: Fetch documents one by one (N+1 but small N).
	// Option B: Update Search to return Document fields too.
	// For "Retrieve-Only API", response expects url/title.
	// Let's go with Option A for now to keep Repository simple, or check if RagChunk has needed info.
	// RagChunk doesn't have metadata like URL/Title.

	// Collect Version IDs to fetch Doc info?
	// Actually we need Document ID from Version ID.
	// Let's optimize: Update `SearchResult` in domain to include `ArticleID`, `DocumentID`.
	// Check `RagChunkRepository.Search` implementation again. It joins everything.
	// I should probably update `SearchResult` to include `ArticleID`.
	// But `RagDocument` stores `ArticleID`. Title/URL/PublishedAt come from... `alt-backend`?
	// Wait, `rag-orchestrator` does NOT store Title/URL/PublishedAt?
	// Let's check `RagDocument` schema.
	// `RagDocument` has `ArticleID`.
	// `UpsertIndexRequest` in OpenAPI has Title, URL, PublishedAt.
	// Does `rag_db` store them?
	// `rag-db` schema in project_rag_detailed.md:
	// "rag_documents（current_version管理）"
	// "rag_chunks（version_id + ordinal + embedding(768)）"
	// It doesn't explicitly mention metadata storage.
	// However, `rag-orchestrator` typically needs to return them.
	// If `rag-orchestrator` doesn't store them, it can't return them.
	// Or maybe `rag_documents` table DOES have them?
	// I should check schema migration file if possible, or `RagDocument` struct.
	// `RagDocument` struct (domain/repository.go) only has ID, ArticleID, CurrentVersionID.
	// This means `rag-orchestrator` does NOT currently store Title/URL.

	// Phase 7 "Response Design": "chunk本文、url/title、published_at、score..."
	// If I don't store them, I can't return them.
	// Phase 2 DDL lists:
	// rag_documents(current_version management)
	// It seems I missed checking if we store metadata.
	// If we don't, we can only return Article ID and `alt-backend` joins it?
	// BUT `alt-backend` calls `retrieve` to get context to display.
	// If `alt-backend` has the articles, it can map ArticleID back.
	// BUT the requirement says "response... url/title".
	// Maybe I should add metadata columns to `rag_documents`?
	// Or `rag_document_versions`?
	// The `Upsert` usecase receives them.
	// Let's check `Upsert` implementation again.
	// It calculates SourceHash from Title+Body.
	// It doesn't seem to save Title/URL.

	// CRITICAL: I need to store Title/URL/PublishedAt if I am to return them.
	// Where? `rag_documents` seems appropriate as they are document-level properties (mostly).
	// Or `rag_document_versions` if they change? (Title might change).
	// Let's look at `RagDocument` struct again.
	// It definitely lacks them.

	// I will update `rag_documents` to store metadata?
	// Or maybe the Phase 7 plan implies `alt-backend` does the merge?
	// "alt-backendの既存UI/機能が retrieve-only を使ってコンテキスト表示できる"
	// If `retrieve` returns chunks, `alt-backend` can display them.
	// `alt-backend` knows the Title/URL for the ArticleID?
	// The `Retrieve` endpoint is `POST /v1/rag/retrieve`.
	// If called by `alt-backend`, it can join.
	// But if called by others?

	// Let's assume for now I return what I have (ArticleID, ChunkText, Score).
	// And maybe I should fetch metadata from `rag_documents` if I add columns?
	// Given I am in "Execution" and didn't plan for DB schema change to add metadata...
	// Maybe `rag_documents` SHOULD have had them.
	// Let me check `migrations` directory to see what is really in DB.

	// 3. Resolve Metadata
	// SearchResult now includes ArticleID and DocumentVersion.
	// We still lack URL/Title/PublishedAt unless we fetch from external source or DB has them.
	// For now, we populate what we have.

	contexts := make([]ContextItem, 0, len(results))

	for _, res := range results {
		contexts = append(contexts, ContextItem{
			ChunkText:       res.Chunk.Content,
			URL:             "", // Placeholder: Not stored in rag-db yet
			Title:           "", // Placeholder: Not stored in rag-db yet
			PublishedAt:     "", // Placeholder: Not stored in rag-db yet
			Score:           res.Score,
			DocumentVersion: res.DocumentVersion,
			ChunkID:         res.Chunk.ID,
		})
	}

	return &RetrieveContextOutput{Contexts: contexts}, nil
}
