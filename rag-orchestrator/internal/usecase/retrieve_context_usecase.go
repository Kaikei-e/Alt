package usecase

import (
	"context"
	"fmt"
	"rag-orchestrator/internal/domain"
	"strings"
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
	chunkRepo    domain.RagChunkRepository
	docRepo      domain.RagDocumentRepository
	encoder      domain.VectorEncoder
	llmClient    domain.LLMClient
	searchClient domain.SearchClient
}

// NewRetrieveContextUsecase creates a new RetrieveContextUsecase.
func NewRetrieveContextUsecase(
	chunkRepo domain.RagChunkRepository,
	docRepo domain.RagDocumentRepository,
	encoder domain.VectorEncoder,
	llmClient domain.LLMClient,
	searchClient domain.SearchClient,
) RetrieveContextUsecase {
	return &retrieveContextUsecase{
		chunkRepo:    chunkRepo,
		docRepo:      docRepo,
		encoder:      encoder,
		llmClient:    llmClient,
		searchClient: searchClient,
	}
}

func (u *retrieveContextUsecase) Execute(ctx context.Context, input RetrieveContextInput) (*RetrieveContextOutput, error) {
	if input.Query == "" {
		return nil, fmt.Errorf("query is empty")
	}

	queries := []string{input.Query}

	// 1a. Check if query is Japanese and translate if so
	if isJapanese(input.Query) {
		translated, err := u.translateQuery(ctx, input.Query)
		if err == nil && translated != "" {
			queries = append(queries, translated)
		} else if err != nil {
			fmt.Printf("Translation failed: %v\n", err)
		}
	}

	// 1b. Search for related tags/terms using SearchClient (Meilisearch)
	if u.searchClient != nil {
		hits, err := u.searchClient.Search(ctx, input.Query)
		if err == nil {
			// Extract tags from top hits (limit to top 3 hits to avoid noise)
			limit := 3
			if len(hits) < limit {
				limit = len(hits)
			}
			tagSet := make(map[string]bool)
			for i := 0; i < limit; i++ {
				for _, tag := range hits[i].Tags {
					if tag != "" {
						tagSet[tag] = true
					}
				}
			}
			// Append unique tags as additional queries
			// Only append if it's not already in queries (simple check)
			for tag := range tagSet {
				if tag != input.Query {
					queries = append(queries, tag)
				}
			}
		} else {
			fmt.Printf("Search client failed: %v\n", err)
		}
	}

	// 1b. Embed all queries
	embeddings, err := u.encoder.Encode(ctx, queries)
	if err != nil {
		return nil, fmt.Errorf("failed to encode queries: %w", err)
	}
	if len(embeddings) != len(queries) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(queries), len(embeddings))
	}

	// 2. Search & Merge
	const searchLimit = 5
	seen := make(map[uuid.UUID]bool)
	var finalResults []domain.SearchResult

	for _, queryVector := range embeddings {
		results, err := u.chunkRepo.Search(ctx, queryVector, input.CandidateArticleIDs, searchLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to search chunks: %w", err)
		}

		for _, res := range results {
			if !seen[res.Chunk.ID] {
				finalResults = append(finalResults, res)
				seen[res.Chunk.ID] = true
			}
		}
	}

	// 3. Resolve Metadata
	contexts := make([]ContextItem, 0, len(finalResults))

	for _, res := range finalResults {
		contexts = append(contexts, ContextItem{
			ChunkText:       res.Chunk.Content,
			URL:             res.URL,
			Title:           res.Title,
			PublishedAt:     res.Chunk.CreatedAt.Format(time.RFC3339),
			Score:           res.Score,
			DocumentVersion: res.DocumentVersion,
			ChunkID:         res.Chunk.ID,
		})
	}

	return &RetrieveContextOutput{Contexts: contexts}, nil
}

func (u *retrieveContextUsecase) translateQuery(ctx context.Context, query string) (string, error) {
	prompt := fmt.Sprintf(`Translate the following Japanese search query into English for cross-lingual information retrieval.
Output ONLY the translated English text. Do not add explanations.

Query: %s`, query)

	// Use a small maxTokens for translation
	resp, err := u.llmClient.Generate(ctx, prompt, 100)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}

func isJapanese(s string) bool {
	for _, r := range s {
		if (r >= '\u3040' && r <= '\u309f') || // Hiragana
			(r >= '\u30a0' && r <= '\u30ff') || // Katakana
			(r >= '\u4e00' && r <= '\u9faf') { // Kanji
			return true
		}
	}
	return false
}
