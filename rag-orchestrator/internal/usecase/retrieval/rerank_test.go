package retrieval_test

import (
	"context"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingReranker records the candidate set it was handed, so a test can
// tell how many distinct candidates actually reached the cross-encoder, and
// scores them in the order received so distinct candidates get distinct
// scores.
type capturingReranker struct {
	got []domain.RerankCandidate
}

func (c *capturingReranker) Rerank(_ context.Context, _ string, candidates []domain.RerankCandidate) ([]domain.RerankResult, error) {
	c.got = candidates
	results := make([]domain.RerankResult, len(candidates))
	for i, cand := range candidates {
		results[i] = domain.RerankResult{ID: cand.ID, Score: 1.0 - float32(i)/10.0}
	}
	return results, nil
}

func (c *capturingReranker) ModelName() string { return "capturing-reranker" }

// TestRerank_BM25OnlyHits_AllReachTheCrossEncoder pins the same identity
// contract as the allocator, one stage earlier. BM25 hits arrive with no chunk
// id (search_indexer_client.go sets ChunkID: "" because Meilisearch indexes
// articles), so keying the candidate map on Chunk.ID collapses the entire
// degraded-mode result set into one candidate — and because the map keeps the
// last write while HitsOriginal is sorted by score descending, the survivor is
// the *worst* hit, whose score is then smeared over every remaining item.
func TestRerank_BM25OnlyHits_AllReachTheCrossEncoder(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID: "bm25-rerank",
		Query:       "why did this happen",
		HitsOriginal: []domain.SearchResult{
			{Chunk: domain.RagChunk{Content: "first"}, ArticleID: "art-1", Title: "Article 1", Score: 0.033},
			{Chunk: domain.RagChunk{Content: "second"}, ArticleID: "art-2", Title: "Article 2", Score: 0.022},
			{Chunk: domain.RagChunk{Content: "third"}, ArticleID: "art-3", Title: "Article 3", Score: 0.011},
		},
	}
	reranker := &capturingReranker{}

	retrieval.Rerank(context.Background(), sc, reranker, retrieval.RerankConfig{
		Enabled: true,
		TopK:    10,
		Timeout: time.Second,
	}, discardLogger())

	assert.Len(t, reranker.got, 3,
		"every BM25-only hit must reach the cross-encoder; collapsing them onto uuid.Nil discards the whole candidate set but one")

	require.Len(t, sc.HitsOriginal, 3)
	assert.True(t, sc.RerankApplied)
	assert.NotEqual(t, sc.HitsOriginal[0].Score, sc.HitsOriginal[2].Score,
		"reranked scores must be per-hit, not one score applied to every hit")
}

// TestRerank_ChunkedHits_StillScoredPerChunk guards the normal path: hits that
// do carry a chunk id must keep being identified by it.
func TestRerank_ChunkedHits_StillScoredPerChunk(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID: "chunked-rerank",
		Query:       "why did this happen",
		HitsOriginal: []domain.SearchResult{
			{Chunk: domain.RagChunk{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Content: "chunk a"}, ArticleID: "art-1", Score: 0.9},
			{Chunk: domain.RagChunk{ID: uuid.MustParse("22222222-2222-2222-2222-222222222222"), Content: "chunk b"}, ArticleID: "art-1", Score: 0.8},
		},
	}
	reranker := &capturingReranker{}

	retrieval.Rerank(context.Background(), sc, reranker, retrieval.RerankConfig{
		Enabled: true,
		TopK:    10,
		Timeout: time.Second,
	}, discardLogger())

	assert.Len(t, reranker.got, 2, "two chunks of the same article are two candidates")
	assert.NotEqual(t, sc.HitsOriginal[0].Score, sc.HitsOriginal[1].Score)
}
