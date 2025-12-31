package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"rag-orchestrator/internal/domain"
	"sort"
	"strings"
	"sync"
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
	Contexts        []ContextItem
	ExpandedQueries []string
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
	chunkRepo     domain.RagChunkRepository
	docRepo       domain.RagDocumentRepository
	encoder       domain.VectorEncoder
	llmClient     domain.LLMClient
	searchClient  domain.SearchClient
	queryExpander domain.QueryExpander
	logger        *slog.Logger
}

// NewRetrieveContextUsecase creates a new RetrieveContextUsecase.
func NewRetrieveContextUsecase(
	chunkRepo domain.RagChunkRepository,
	docRepo domain.RagDocumentRepository,
	encoder domain.VectorEncoder,
	llmClient domain.LLMClient,
	searchClient domain.SearchClient,
	queryExpander domain.QueryExpander,
	logger *slog.Logger,
) RetrieveContextUsecase {
	return &retrieveContextUsecase{
		chunkRepo:     chunkRepo,
		docRepo:       docRepo,
		encoder:       encoder,
		llmClient:     llmClient,
		searchClient:  searchClient,
		queryExpander: queryExpander,
		logger:        logger,
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

	// 1a. Expand query (translate & variations) using LLM
	expandedQueries, err := u.expandQuery(ctx, input.Query)
	if err == nil && len(expandedQueries) > 0 {
		queries = append(queries, expandedQueries...)
		u.logger.Info("query_expanded",
			slog.String("retrieval_id", retrievalID),
			slog.String("original", input.Query),
			slog.Any("expanded", expandedQueries))
	} else if err != nil {
		u.logger.Warn("expansion_failed",
			slog.String("retrieval_id", retrievalID),
			slog.String("query", input.Query),
			slog.String("error", err.Error()))
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
				exists := false
				for _, existing := range queries {
					if existing == tag {
						exists = true
						break
					}
				}
				if !exists {
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

	// 2. Search & Merge using Quota Strategy with Parallel Vector Search
	const (
		searchLimit   = 50
		rrfK          = 60.0
		quotaOriginal = 5
		quotaExpanded = 5
	)

	// Parallel vector search for all query embeddings
	type searchResult struct {
		index   int
		results []domain.SearchResult
		err     error
	}

	searchStart := time.Now()
	resultsChan := make(chan searchResult, len(embeddings))
	var wg sync.WaitGroup

	for i, queryVector := range embeddings {
		wg.Add(1)
		go func(idx int, qv []float32) {
			defer wg.Done()
			results, err := u.chunkRepo.Search(ctx, qv, input.CandidateArticleIDs, searchLimit)
			resultsChan <- searchResult{index: idx, results: results, err: err}
		}(i, queryVector)
	}

	// Wait for all searches to complete, then close channel
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results from channel
	allResults := make([][]domain.SearchResult, len(embeddings))
	var searchErr error
	for sr := range resultsChan {
		if sr.err != nil && searchErr == nil {
			searchErr = sr.err
		}
		allResults[sr.index] = sr.results
	}
	if searchErr != nil {
		return nil, fmt.Errorf("failed to search chunks: %w", searchErr)
	}

	searchDuration := time.Since(searchStart)
	u.logger.Info("parallel_vector_search_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("query_count", len(embeddings)),
		slog.Int64("duration_ms", searchDuration.Milliseconds()))

	var hitsOriginal []domain.SearchResult

	// Map to track unique chunks and their accumulated RRF score for EXPANDED queries
	type chunkData struct {
		Item     ContextItem
		RRFScore float64
	}
	chunksMapExpanded := make(map[uuid.UUID]*chunkData)

	// Process collected results
	for i, results := range allResults {
		if i == 0 {
			// Original Query (Index 0)
			hitsOriginal = results
		} else {
			// Expanded Queries (Index 1+) - Accumulate using RRF
			for rank, res := range results {
				if _, exists := chunksMapExpanded[res.Chunk.ID]; !exists {
					chunksMapExpanded[res.Chunk.ID] = &chunkData{
						Item: ContextItem{
							ChunkText:       res.Chunk.Content,
							URL:             res.URL,
							Title:           res.Title,
							PublishedAt:     res.Chunk.CreatedAt.Format(time.RFC3339),
							DocumentVersion: res.DocumentVersion,
							ChunkID:         res.Chunk.ID,
							Score:           res.Score,
						},
						RRFScore: 0,
					}
				}
				chunksMapExpanded[res.Chunk.ID].RRFScore += 1.0 / (rrfK + float64(rank+1))
			}
		}
	}

	// Prepare Expanded list sorted by RRF
	hitsExpanded := make([]ContextItem, 0, len(chunksMapExpanded))
	for _, data := range chunksMapExpanded {
		hitsExpanded = append(hitsExpanded, data.Item)
	}
	sort.Slice(hitsExpanded, func(i, j int) bool {
		return chunksMapExpanded[hitsExpanded[i].ChunkID].RRFScore > chunksMapExpanded[hitsExpanded[j].ChunkID].RRFScore
	})

	// Log top expanded hits for debugging
	debugLimit := 5
	if len(hitsExpanded) < debugLimit {
		debugLimit = len(hitsExpanded)
	}
	if debugLimit > 0 {
		var debugLog []map[string]interface{}
		for i := 0; i < debugLimit; i++ {
			debugLog = append(debugLog, map[string]interface{}{
				"title": hitsExpanded[i].Title,
				"url":   hitsExpanded[i].URL,
				"score": hitsExpanded[i].Score,
				"rrf":   chunksMapExpanded[hitsExpanded[i].ChunkID].RRFScore,
			})
		}
		u.logger.Info("expanded_query_hits_debug",
			slog.String("retrieval_id", retrievalID),
			slog.Any("top_hits", debugLog))
	} else {
		u.logger.Info("expanded_query_hits_debug",
			slog.String("retrieval_id", retrievalID),
			slog.String("msg", "no hits for expanded queries"))
	}

	// 3. Resolve Metadata & Merge with Quota
	contexts := make([]ContextItem, 0, quotaOriginal+quotaExpanded)
	seen := make(map[uuid.UUID]bool)

	// Add Top N from Original
	countOriginal := 0
	for _, res := range hitsOriginal {
		if countOriginal >= quotaOriginal {
			break
		}
		if !seen[res.Chunk.ID] {
			contexts = append(contexts, ContextItem{
				ChunkText:       res.Chunk.Content,
				URL:             res.URL,
				Title:           res.Title,
				PublishedAt:     res.Chunk.CreatedAt.Format(time.RFC3339),
				Score:           res.Score,
				DocumentVersion: res.DocumentVersion,
				ChunkID:         res.Chunk.ID,
			})
			seen[res.Chunk.ID] = true
			countOriginal++
		}
	}

	// Add Top M from Expanded
	countExpanded := 0

	// Pass 1: Prioritize English/Non-Japanese documents
	for _, item := range hitsExpanded {
		if countExpanded >= quotaExpanded {
			break
		}
		if seen[item.ChunkID] {
			continue
		}
		// If title is NOT Japanese (contains no CJK), prioritize it
		if !isJapanese(item.Title) {
			contexts = append(contexts, item)
			seen[item.ChunkID] = true
			countExpanded++
		}
	}

	// Pass 2: Fill remaining quota with other documents (e.g. Japanese)
	for _, item := range hitsExpanded {
		if countExpanded >= quotaExpanded {
			break
		}
		if seen[item.ChunkID] {
			continue
		}
		contexts = append(contexts, item)
		seen[item.ChunkID] = true
		countExpanded++
	}

	u.logger.Info("vector_search_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("original_hits", len(hitsOriginal)),
		slog.Int("expanded_hits_unique", len(hitsExpanded)),
		slog.Int("final_contexts", len(contexts)))

	retrievalDuration := time.Since(retrievalStart)
	u.logger.Info("retrieval_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("contexts_returned", len(contexts)),
		slog.Int64("duration_ms", retrievalDuration.Milliseconds()))

	var expandedQueriesRet []string
	if len(expandedQueries) > 0 {
		expandedQueriesRet = expandedQueries
	}

	return &RetrieveContextOutput{
		Contexts:        contexts,
		ExpandedQueries: expandedQueriesRet,
	}, nil
}

func (u *retrieveContextUsecase) expandQuery(ctx context.Context, query string) ([]string, error) {
	// Use the dedicated QueryExpander (news-creator) for faster GPU-accelerated expansion
	if u.queryExpander != nil {
		// Generate 1 Japanese + 3 English query variations
		expansions, err := u.queryExpander.ExpandQuery(ctx, query, 1, 3)
		if err != nil {
			u.logger.Warn("query_expansion_via_news_creator_failed",
				slog.String("query", query),
				slog.String("error", err.Error()))
			// Fall back to legacy LLM-based expansion
			return u.expandQueryLegacy(ctx, query)
		}
		return expansions, nil
	}

	// Fallback to legacy expansion if no QueryExpander is configured
	return u.expandQueryLegacy(ctx, query)
}

// expandQueryLegacy uses the LLMClient for query expansion (legacy fallback).
func (u *retrieveContextUsecase) expandQueryLegacy(ctx context.Context, query string) ([]string, error) {
	currentDate := time.Now().Format("2006-01-02")
	prompt := fmt.Sprintf(`You are an expert search query generator.
Current Date: %s

Generate 3 to 5 diverse English search queries to find information related to the user's input.
If the input is Japanese, translate it and also generate variations.
If the user specifies a time (e.g., "December" or "this month"), interpret it based on the Current Date.
Focus on different aspects like main keywords, synonyms, and specific events.
Output ONLY the generated queries, one per line. Do not add numbering or bullets or explanations.

User Input: %s`, currentDate, query)

	// Use a small maxTokens for expansion
	resp, err := u.llmClient.Generate(ctx, prompt, 200)
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
