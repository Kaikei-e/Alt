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
	reranker      domain.Reranker     // Optional: cross-encoder reranking
	bm25Searcher  domain.BM25Searcher // Optional: BM25 search for hybrid fusion
	config        RetrievalConfig
	logger        *slog.Logger
}

// RetrieveContextOption is a functional option for configuring the usecase.
type RetrieveContextOption func(*retrieveContextUsecase)

// WithReranker sets an optional cross-encoder reranker.
// If not set or nil, reranking is skipped.
func WithReranker(r domain.Reranker) RetrieveContextOption {
	return func(u *retrieveContextUsecase) {
		u.reranker = r
	}
}

// WithBM25Searcher sets an optional BM25 searcher for hybrid search fusion.
// If not set or nil, pure vector search is used.
func WithBM25Searcher(s domain.BM25Searcher) RetrieveContextOption {
	return func(u *retrieveContextUsecase) {
		u.bm25Searcher = s
	}
}

// NewRetrieveContextUsecase creates a new RetrieveContextUsecase.
// If config is zero-valued, defaults are used (research-backed values).
func NewRetrieveContextUsecase(
	chunkRepo domain.RagChunkRepository,
	docRepo domain.RagDocumentRepository,
	encoder domain.VectorEncoder,
	llmClient domain.LLMClient,
	searchClient domain.SearchClient,
	queryExpander domain.QueryExpander,
	config RetrievalConfig,
	logger *slog.Logger,
	opts ...RetrieveContextOption,
) RetrieveContextUsecase {
	// Apply defaults if config is zero-valued
	if config.SearchLimit == 0 {
		config = DefaultRetrievalConfig()
	}
	u := &retrieveContextUsecase{
		chunkRepo:     chunkRepo,
		docRepo:       docRepo,
		encoder:       encoder,
		llmClient:     llmClient,
		searchClient:  searchClient,
		queryExpander: queryExpander,
		config:        config,
		logger:        logger,
	}
	for _, opt := range opts {
		opt(u)
	}
	return u
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
	// Use configurable parameters (research-backed defaults from RetrievalConfig)
	searchLimit := u.config.SearchLimit
	rrfK := u.config.RRFK
	quotaOriginal := u.config.QuotaOriginal
	quotaExpanded := u.config.QuotaExpanded

	// Parallel vector search for all query embeddings
	type searchResult struct {
		index   int
		results []domain.SearchResult
		err     error
	}

	searchStart := time.Now()
	resultsChan := make(chan searchResult, len(embeddings))
	var wg sync.WaitGroup

	// Choose search strategy based on whether we have candidate article IDs
	// - CandidateArticleIDs present: Morning Letter use case (time-bounded search)
	// - CandidateArticleIDs empty: Augur use case (full corpus search)
	hasCandidateArticles := len(input.CandidateArticleIDs) > 0

	for i, queryVector := range embeddings {
		wg.Add(1)
		go func(idx int, qv []float32) {
			defer wg.Done()
			var results []domain.SearchResult
			var err error
			if hasCandidateArticles {
				// Morning Letter: Search within specific articles
				results, err = u.chunkRepo.SearchWithinArticles(ctx, qv, input.CandidateArticleIDs, searchLimit)
			} else {
				// Augur: Search across all chunks
				results, err = u.chunkRepo.Search(ctx, qv, searchLimit)
			}
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

	// 2b. Hybrid Search: Fuse BM25 results with vector results (if enabled)
	var bm25Results []domain.BM25SearchResult
	if u.config.HybridSearch.Enabled && u.bm25Searcher != nil {
		bm25Start := time.Now()
		var bm25Err error
		bm25Results, bm25Err = u.bm25Searcher.SearchBM25(ctx, input.Query, u.config.HybridSearch.BM25Limit)
		bm25Duration := time.Since(bm25Start)

		if bm25Err != nil {
			u.logger.Warn("hybrid_bm25_search_failed",
				slog.String("retrieval_id", retrievalID),
				slog.String("error", bm25Err.Error()),
				slog.Int64("duration_ms", bm25Duration.Milliseconds()))
			// Continue with vector-only results
		} else {
			u.logger.Info("hybrid_bm25_search_completed",
				slog.String("retrieval_id", retrievalID),
				slog.Int("bm25_hits", len(bm25Results)),
				slog.Int64("duration_ms", bm25Duration.Milliseconds()))

			// Apply RRF fusion to original query results (index 0)
			if len(allResults) > 0 && len(bm25Results) > 0 {
				allResults[0] = u.fuseHybridResults(allResults[0], bm25Results, rrfK, retrievalID)
			}
		}
	}

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

	// 3. Apply Re-ranking (if enabled and reranker is available)
	if u.config.Reranking.Enabled && u.reranker != nil {
		rerankStart := time.Now()

		// Prepare candidates from all unique hits (original + expanded)
		candidateMap := make(map[uuid.UUID]domain.SearchResult)
		for _, res := range hitsOriginal {
			candidateMap[res.Chunk.ID] = res
		}
		for id, data := range chunksMapExpanded {
			if _, exists := candidateMap[id]; !exists {
				candidateMap[id] = domain.SearchResult{
					Chunk: domain.RagChunk{
						ID:        id,
						Content:   data.Item.ChunkText,
						CreatedAt: time.Time{}, // Will use data.Item.PublishedAt
					},
					Score:           data.Item.Score,
					Title:           data.Item.Title,
					URL:             data.Item.URL,
					DocumentVersion: data.Item.DocumentVersion,
				}
			}
		}

		// Convert to rerank candidates
		candidates := make([]domain.RerankCandidate, 0, len(candidateMap))
		for id, res := range candidateMap {
			candidates = append(candidates, domain.RerankCandidate{
				ID:      id.String(),
				Content: res.Chunk.Content,
				Score:   res.Score,
			})
		}

		// Call reranker with timeout from config
		rerankCtx, cancel := context.WithTimeout(ctx, u.config.Reranking.Timeout)
		reranked, err := u.reranker.Rerank(rerankCtx, input.Query, candidates)
		cancel()

		rerankDuration := time.Since(rerankStart)

		if err != nil {
			// Fallback: log warning and continue with original scores
			u.logger.Warn("reranking_failed_using_original_scores",
				slog.String("retrieval_id", retrievalID),
				slog.String("error", err.Error()),
				slog.Int64("duration_ms", rerankDuration.Milliseconds()))
		} else {
			u.logger.Info("reranking_completed",
				slog.String("retrieval_id", retrievalID),
				slog.Int("candidate_count", len(candidates)),
				slog.Int("reranked_count", len(reranked)),
				slog.String("model", u.reranker.ModelName()),
				slog.Int64("duration_ms", rerankDuration.Milliseconds()))

			// Apply reranked scores to hitsOriginal and hitsExpanded
			rerankScores := make(map[uuid.UUID]float32)
			for _, r := range reranked {
				id, _ := uuid.Parse(r.ID)
				rerankScores[id] = r.Score
			}

			// Update original hits scores
			for i := range hitsOriginal {
				if score, ok := rerankScores[hitsOriginal[i].Chunk.ID]; ok {
					hitsOriginal[i].Score = score
				}
			}
			// Sort by reranked score
			sort.Slice(hitsOriginal, func(i, j int) bool {
				return hitsOriginal[i].Score > hitsOriginal[j].Score
			})

			// Update expanded hits scores
			for i := range hitsExpanded {
				if score, ok := rerankScores[hitsExpanded[i].ChunkID]; ok {
					hitsExpanded[i].Score = score
				}
			}
			// Sort by reranked score
			sort.Slice(hitsExpanded, func(i, j int) bool {
				return hitsExpanded[i].Score > hitsExpanded[j].Score
			})
		}
	}

	// 4. Resolve Metadata & Merge with Quota
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

// fuseHybridResults merges vector search results with BM25 results using RRF.
// Research basis:
// - EMNLP 2024: RRF fusion outperforms linear combination for most use cases
// - Formula: RRF(d) = sum(1 / (k + rank(d))) across both result lists
func (u *retrieveContextUsecase) fuseHybridResults(
	vectorResults []domain.SearchResult,
	bm25Results []domain.BM25SearchResult,
	rrfK float64,
	retrievalID string,
) []domain.SearchResult {
	// Map to accumulate RRF scores by article ID
	type fusedResult struct {
		vectorResult *domain.SearchResult
		rrfScore     float64
	}
	fusedMap := make(map[string]*fusedResult)

	// Process vector search results
	for i, vr := range vectorResults {
		articleID := vr.ArticleID
		if _, exists := fusedMap[articleID]; !exists {
			vrCopy := vr // Copy to avoid pointer issues
			fusedMap[articleID] = &fusedResult{
				vectorResult: &vrCopy,
				rrfScore:     0,
			}
		}
		// RRF score: 1 / (k + rank), rank is 1-indexed
		fusedMap[articleID].rrfScore += 1.0 / (rrfK + float64(i+1))
	}

	// Process BM25 results
	for _, br := range bm25Results {
		articleID := br.ArticleID
		if existing, exists := fusedMap[articleID]; exists {
			// Article exists in vector results, add BM25 RRF score
			existing.rrfScore += 1.0 / (rrfK + float64(br.Rank))
		} else {
			// Article only in BM25 results - create placeholder
			// Note: This is less common since we primarily rely on vector search
			// for chunk-level content, but BM25 can surface articles not in vector results
			fusedMap[articleID] = &fusedResult{
				vectorResult: nil, // No vector result for this article
				rrfScore:     1.0 / (rrfK + float64(br.Rank)),
			}
		}
	}

	// Convert back to slice and sort by fused RRF score
	results := make([]domain.SearchResult, 0, len(fusedMap))
	for _, fr := range fusedMap {
		if fr.vectorResult != nil {
			// Use vector result but update score to reflect fusion
			result := *fr.vectorResult
			result.Score = float32(fr.rrfScore) // Replace with RRF score
			results = append(results, result)
		}
		// Skip BM25-only results as we don't have chunk content for them
	}

	// Sort by fused RRF score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	u.logger.Info("hybrid_rrf_fusion_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("vector_count", len(vectorResults)),
		slog.Int("bm25_count", len(bm25Results)),
		slog.Int("fused_count", len(results)))

	return results
}
