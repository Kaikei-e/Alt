package retrieval

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"rag-orchestrator/internal/domain"

	"golang.org/x/sync/errgroup"
)

// ExpandQueries runs query expansion, tag search, and original embedding in parallel (Stage 1).
func ExpandQueries(
	ctx context.Context,
	sc *StageContext,
	queryExpander domain.QueryExpander,
	llmClient domain.LLMClient,
	searchClient domain.SearchClient,
	encoder domain.VectorEncoder,
	logger *slog.Logger,
) error {
	g, gctx := errgroup.WithContext(ctx)

	// goroutine A: Query Expansion
	g.Go(func() error {
		expanded, err := expandQuery(gctx, sc.Query, queryExpander, llmClient, logger)
		if err != nil {
			logger.Warn("expansion_failed",
				slog.String("retrieval_id", sc.RetrievalID),
				slog.String("query", sc.Query),
				slog.String("error", err.Error()))
			return nil // non-fatal
		}
		if len(expanded) > 0 {
			sc.ExpandedQueries = expanded
			logger.Info("query_expanded",
				slog.String("retrieval_id", sc.RetrievalID),
				slog.String("original", sc.Query),
				slog.Any("expanded", expanded))
		}
		return nil
	})

	// goroutine B: Tag Search
	g.Go(func() error {
		if searchClient == nil {
			return nil
		}
		tagSearchStart := time.Now()
		hits, err := searchClient.Search(gctx, sc.Query)
		tagSearchDuration := time.Since(tagSearchStart)

		if err != nil {
			logger.Warn("tag_search_failed",
				slog.String("retrieval_id", sc.RetrievalID),
				slog.String("error", err.Error()))
			return nil // non-fatal
		}

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
		for tag := range tagSet {
			if tag != sc.Query {
				sc.TagQueries = append(sc.TagQueries, tag)
			}
		}

		logger.Info("tag_search_completed",
			slog.String("retrieval_id", sc.RetrievalID),
			slog.Int("hits_found", len(hits)),
			slog.Int("tags_extracted", len(sc.TagQueries)),
			slog.Int64("duration_ms", tagSearchDuration.Milliseconds()))
		return nil
	})

	// goroutine C: Original Query Embedding
	g.Go(func() error {
		embeddings, err := encoder.Encode(gctx, []string{sc.Query})
		if err != nil {
			return fmt.Errorf("failed to encode original query: %w", err)
		}
		if len(embeddings) == 0 {
			return fmt.Errorf("no embedding returned for original query")
		}
		sc.OriginalEmbedding = embeddings[0]
		return nil
	})

	return g.Wait()
}

func expandQuery(ctx context.Context, query string, queryExpander domain.QueryExpander, llmClient domain.LLMClient, logger *slog.Logger) ([]string, error) {
	if queryExpander == nil {
		return expandQueryLegacy(ctx, query, llmClient)
	}

	type result struct {
		queries []string
		err     error
		source  string
	}
	ch := make(chan result, 2)

	go func() {
		expansions, err := queryExpander.ExpandQuery(ctx, query, 1, 3)
		ch <- result{expansions, err, "news-creator"}
	}()

	go func() {
		expansions, err := expandQueryLegacy(ctx, query, llmClient)
		ch <- result{expansions, err, "ollama-legacy"}
	}()

	var lastErr error
	for range 2 {
		r := <-ch
		if r.err == nil && len(r.queries) > 0 {
			logger.Info("query_expansion_completed",
				slog.String("source", r.source),
				slog.Int("count", len(r.queries)))
			return r.queries, nil
		}
		if r.err != nil {
			logger.Warn("query_expansion_source_failed",
				slog.String("source", r.source),
				slog.String("error", r.err.Error()))
			lastErr = r.err
		}
	}
	return nil, fmt.Errorf("all expansion methods failed: %w", lastErr)
}

func expandQueryLegacy(ctx context.Context, query string, llmClient domain.LLMClient) ([]string, error) {
	currentDate := time.Now().Format("2006-01-02")
	prompt := fmt.Sprintf(`You are an expert search query generator.
Current Date: %s

Generate 3 to 5 diverse English search queries to find information related to the user's input.
If the input is Japanese, translate it and also generate variations.
If the user specifies a time (e.g., "December" or "this month"), interpret it based on the Current Date.
Focus on different aspects like main keywords, synonyms, and specific events.
Output ONLY the generated queries, one per line. Do not add numbering or bullets or explanations.

User Input: %s`, currentDate, query)

	resp, err := llmClient.Generate(ctx, prompt, 200)
	if err != nil {
		return nil, err
	}

	rawLines := strings.Split(resp.Text, "\n")
	var expansions []string
	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			expansions = append(expansions, trimmed)
		}
	}
	return expansions, nil
}
