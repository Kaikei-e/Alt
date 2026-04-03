package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"rag-orchestrator/internal/domain"
)

// ChunkSearcher searches chunks by vector embedding. Abstracts pgvector access.
type ChunkSearcher interface {
	SearchByVector(ctx context.Context, embedding []float32, limit int) ([]ContextItem, error)
}

// HyDEGenerator implements Hypothetical Document Embeddings.
// Generates a hypothetical answer, embeds it, and retrieves similar real documents.
// Used as an adaptive fallback when standard retrieval has insufficient coverage.
type HyDEGenerator struct {
	llmClient domain.LLMClient
	encoder   domain.VectorEncoder
	searcher  ChunkSearcher
	logger    *slog.Logger
}

// NewHyDEGenerator creates a new HyDE generator.
func NewHyDEGenerator(
	llmClient domain.LLMClient,
	encoder domain.VectorEncoder,
	searcher ChunkSearcher,
	logger *slog.Logger,
) *HyDEGenerator {
	return &HyDEGenerator{
		llmClient: llmClient,
		encoder:   encoder,
		searcher:  searcher,
		logger:    logger,
	}
}

// GenerateAndRetrieve creates a hypothetical answer, embeds it, and retrieves
// similar chunks. Chunks in existingIDs are excluded from results.
// Returns empty slice (not error) on LLM or embedding failure for graceful degradation.
func (h *HyDEGenerator) GenerateAndRetrieve(
	ctx context.Context,
	query string,
	existingIDs map[string]bool,
	limit int,
) ([]ContextItem, error) {
	// 1. Generate hypothetical answer
	prompt := fmt.Sprintf(
		"Write a brief, factual paragraph (100-150 words) answering: %s\n"+
			"Use specific terms, names, and concepts that would appear in real news articles.\n"+
			"Output ONLY the paragraph. No introduction.",
		query,
	)
	resp, err := h.llmClient.Generate(ctx, prompt, 256)
	if err != nil {
		h.log("hyde_generation_failed", slog.String("error", err.Error()))
		return nil, nil // graceful: return empty, not error
	}

	if resp.Text == "" {
		h.log("hyde_empty_response")
		return nil, nil
	}

	h.log("hyde_generated", slog.Int("length", len(resp.Text)))

	// 2. Embed the hypothetical document
	embeddings, err := h.encoder.Encode(ctx, []string{resp.Text})
	if err != nil || len(embeddings) == 0 {
		h.log("hyde_embedding_failed", slog.String("error", fmt.Sprintf("%v", err)))
		return nil, nil
	}
	embedding := embeddings[0]

	// 3. Search with HyDE embedding
	chunks, err := h.searcher.SearchByVector(ctx, embedding, limit+len(existingIDs))
	if err != nil {
		h.log("hyde_search_failed", slog.String("error", err.Error()))
		return nil, nil
	}

	// 4. Filter out existing chunks
	var filtered []ContextItem
	for _, c := range chunks {
		if existingIDs != nil && existingIDs[c.ChunkID.String()] {
			continue
		}
		filtered = append(filtered, c)
		if len(filtered) >= limit {
			break
		}
	}

	h.log("hyde_retrieved", slog.Int("new_chunks", len(filtered)))
	return filtered, nil
}

func (h *HyDEGenerator) log(msg string, attrs ...slog.Attr) {
	if h.logger == nil {
		return
	}
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	h.logger.Info(msg, args...)
}
