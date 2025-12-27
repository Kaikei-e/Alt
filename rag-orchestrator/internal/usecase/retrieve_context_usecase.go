package usecase

import (
	"context"
	"fmt"
	"log/slog"
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
	logger       *slog.Logger
}

// NewRetrieveContextUsecase creates a new RetrieveContextUsecase.
func NewRetrieveContextUsecase(
	chunkRepo domain.RagChunkRepository,
	docRepo domain.RagDocumentRepository,
	encoder domain.VectorEncoder,
	llmClient domain.LLMClient,
	searchClient domain.SearchClient,
	logger *slog.Logger,
) RetrieveContextUsecase {
	return &retrieveContextUsecase{
		chunkRepo:    chunkRepo,
		docRepo:      docRepo,
		encoder:      encoder,
		llmClient:    llmClient,
		searchClient: searchClient,
		logger:       logger,
	}
}

func (u *retrieveContextUsecase) Execute(ctx context.Context, input RetrieveContextInput) (*RetrieveContextOutput, error) {
	if input.Query == "" {
		return nil, fmt.Errorf("query is empty")
	}

	retrievalStart := time.Now()
	retrievalID := uuid.NewString()
	u.logger.Info("retrieval_started",
		slog.String("retrieval_id", retrievalID),
		slog.String("query", input.Query),
		slog.Int("candidate_articles", len(input.CandidateArticleIDs)))

	queries := []string{input.Query}

	// 1a. Check if query is Japanese and translate if so
	if isJapanese(input.Query) {
		translated, err := u.translateQuery(ctx, input.Query)
		if err == nil && translated != "" {
			queries = append(queries, translated)
			u.logger.Info("query_translated",
				slog.String("retrieval_id", retrievalID),
				slog.String("original", input.Query),
				slog.String("translated", translated))
		} else if err != nil {
			u.logger.Warn("translation_failed",
				slog.String("retrieval_id", retrievalID),
				slog.String("query", input.Query),
				slog.String("error", err.Error()))
		}
	}

	// 1b. Search for related tags/terms using SearchClient (Meilisearch)
	if u.searchClient != nil {
		tagSearchStart := time.Now()
		hits, err := u.searchClient.Search(ctx, input.Query)
		tagSearchDuration := time.Since(tagSearchStart)

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
			tagCount := 0
			for tag := range tagSet {
				if tag != input.Query {
					queries = append(queries, tag)
					tagCount++
				}
			}

			u.logger.Info("tag_search_completed",
				slog.String("retrieval_id", retrievalID),
				slog.Int("hits_found", len(hits)),
				slog.Int("tags_extracted", tagCount),
				slog.Int64("duration_ms", tagSearchDuration.Milliseconds()))
		} else {
			u.logger.Warn("tag_search_failed",
				slog.String("retrieval_id", retrievalID),
				slog.String("error", err.Error()))
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

	u.logger.Info("queries_encoded",
		slog.String("retrieval_id", retrievalID),
		slog.Int("query_count", len(queries)),
		slog.Any("queries", queries))

	// 2. Search & Merge
	const searchLimit = 10
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

	u.logger.Info("vector_search_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("total_results", len(finalResults)),
		slog.Int("unique_chunks", len(seen)))

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

	retrievalDuration := time.Since(retrievalStart)
	u.logger.Info("retrieval_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("contexts_returned", len(contexts)),
		slog.Int64("duration_ms", retrievalDuration.Milliseconds()))

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
