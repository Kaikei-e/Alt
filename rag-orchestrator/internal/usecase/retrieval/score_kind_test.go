package retrieval_test

import (
	"context"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestFuseResults_BM25OnlyDegraded_TagsScoresAsBM25 pins the root cause of the
// 2026-04-11 fallback-rate regression: promoted BM25 hits carried a lexical
// score in the same float32 field that the relevance gate reads as a
// cross-encoder score. The number alone cannot say which space it belongs to,
// so the space has to travel with it.
func TestFuseResults_BM25OnlyDegraded_TagsScoresAsBM25(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:       "score-kind-bm25",
		Query:             "test query",
		OriginalEmbedding: nil,
		BM25Results: []domain.BM25SearchResult{
			{ArticleID: "art-1", Content: "lexical hit", Title: "Article 1", Rank: 1, Score: 12.5},
			{ArticleID: "art-2", Content: "another", Title: "Article 2", Rank: 2, Score: 9.0},
		},
		SearchLimit: 50,
		RRFK:        60.0,
	}

	require.NoError(t, retrieval.FuseResults(context.Background(), sc, new(MockRagChunkRepository), nil, false, discardLogger()))

	require.Len(t, sc.HitsOriginal, 2)
	for _, hit := range sc.HitsOriginal {
		assert.Equal(t, domain.ScoreKindBM25, hit.ScoreKind,
			"a promoted BM25 hit must declare the lexical score space it lives in")
	}
}

// TestFuseResults_HybridFusion_TagsScoresAsRRF covers the mode where the
// application-level RRF replaces both input scores with a rank-derived value
// around 1/(k+rank) — a number that looks like a terrible cosine similarity.
func TestFuseResults_HybridFusion_TagsScoresAsRRF(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:       "score-kind-rrf",
		Query:             "test query",
		OriginalEmbedding: []float32{0.1, 0.2},
		OriginalResults: []domain.SearchResult{
			{
				Chunk:     domain.RagChunk{ID: uuid.New(), Content: "vector hit", CreatedAt: time.Now()},
				Score:     0.9,
				ScoreKind: domain.ScoreKindVector,
				ArticleID: "art-1",
				Title:     "Article 1",
			},
		},
		BM25Results: []domain.BM25SearchResult{{ArticleID: "art-1", Rank: 1, Score: 10.0}},
		SearchLimit: 50,
		RRFK:        60.0,
	}

	require.NoError(t, retrieval.FuseResults(context.Background(), sc, new(MockRagChunkRepository), nil, false, discardLogger()))

	require.Len(t, sc.HitsOriginal, 1)
	assert.Equal(t, domain.ScoreKindRRF, sc.HitsOriginal[0].ScoreKind,
		"RRF fusion rewrites Score into rank space, so the kind must follow it")
}

// TestFuseResults_VectorOnly_KeepsVectorScoreKind guards the untouched path:
// with no lexical arm the original hits keep the repository's cosine score and
// must keep saying so.
func TestFuseResults_VectorOnly_KeepsVectorScoreKind(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:       "score-kind-vector",
		Query:             "test query",
		OriginalEmbedding: []float32{0.1, 0.2},
		OriginalResults: []domain.SearchResult{
			{
				Chunk:     domain.RagChunk{ID: uuid.New(), Content: "vector hit", CreatedAt: time.Now()},
				Score:     0.9,
				ScoreKind: domain.ScoreKindVector,
				ArticleID: "art-1",
			},
		},
		SearchLimit: 50,
		RRFK:        60.0,
	}

	require.NoError(t, retrieval.FuseResults(context.Background(), sc, new(MockRagChunkRepository), nil, false, discardLogger()))

	require.Len(t, sc.HitsOriginal, 1)
	assert.Equal(t, domain.ScoreKindVector, sc.HitsOriginal[0].ScoreKind)
}

// TestFuseResults_ExpandedHits_CarryTheFusedRRFScore pins that the expanded
// list ranks on the value it is sorted by. It used to be sorted by an internal
// RRF sum while Score held the per-query similarity of whichever query hit the
// chunk first, so Allocate's own sort silently undid the fusion.
func TestFuseResults_ExpandedHits_CarryTheFusedRRFScore(t *testing.T) {
	repo := new(MockRagChunkRepository)
	sharedID := uuid.New()
	loneID := uuid.New()

	sc := &retrieval.StageContext{
		RetrievalID:          "score-kind-expanded",
		Query:                "test query",
		OriginalEmbedding:    []float32{0.1, 0.2},
		AdditionalEmbeddings: [][]float32{{0.3, 0.4}, {0.5, 0.6}},
		AdditionalQueries:    []string{"expanded 1", "expanded 2"},
		SearchLimit:          50,
		RRFK:                 60.0,
	}

	// The lone chunk wins on raw similarity but is found by one query only;
	// the shared chunk is found by both, so RRF must rank it first.
	repo.On("Search", mock.Anything, []float32{0.3, 0.4}, 50).Return([]domain.SearchResult{
		{Chunk: domain.RagChunk{ID: sharedID, Content: "shared", CreatedAt: time.Now()}, Score: 0.5, ScoreKind: domain.ScoreKindVector, ArticleID: "art-shared"},
	}, nil)
	repo.On("Search", mock.Anything, []float32{0.5, 0.6}, 50).Return([]domain.SearchResult{
		{Chunk: domain.RagChunk{ID: loneID, Content: "lone", CreatedAt: time.Now()}, Score: 0.99, ScoreKind: domain.ScoreKindVector, ArticleID: "art-lone"},
		{Chunk: domain.RagChunk{ID: sharedID, Content: "shared", CreatedAt: time.Now()}, Score: 0.5, ScoreKind: domain.ScoreKindVector, ArticleID: "art-shared"},
	}, nil)

	require.NoError(t, retrieval.FuseResults(context.Background(), sc, repo, nil, false, discardLogger()))

	require.Len(t, sc.HitsExpanded, 2)
	assert.Equal(t, sharedID, sc.HitsExpanded[0].ChunkID,
		"the chunk both expanded queries found must outrank the single-query chunk")
	for _, hit := range sc.HitsExpanded {
		assert.Equal(t, domain.ScoreKindRRF, hit.ScoreKind)
	}
	assert.Greater(t, sc.HitsExpanded[0].Score, sc.HitsExpanded[1].Score,
		"Score must hold the fused RRF value the list is ordered by")
}

// TestRerank_Success_TagsScoresAsRerank is the other half of the contract: only
// a cross-encoder score may be compared against the gate's calibrated
// thresholds, so only the rerank stage may stamp that kind.
func TestRerank_Success_TagsScoresAsRerank(t *testing.T) {
	chunkID := uuid.New()
	sc := &retrieval.StageContext{
		RetrievalID: "score-kind-rerank",
		Query:       "why did this happen",
		HitsOriginal: []domain.SearchResult{
			{Chunk: domain.RagChunk{ID: chunkID, Content: "first"}, ArticleID: "art-1", Score: 0.033, ScoreKind: domain.ScoreKindRRF},
		},
		HitsExpanded: []retrieval.ContextItem{
			{ChunkID: uuid.New(), ChunkText: "second", ArticleID: "art-2", Score: 0.022, ScoreKind: domain.ScoreKindRRF},
		},
	}

	retrieval.Rerank(context.Background(), sc, &capturingReranker{}, retrieval.RerankConfig{
		Enabled:       true,
		TopK:          10,
		MaxCandidates: 40,
		Timeout:       time.Second,
	}, discardLogger())

	require.NotEmpty(t, sc.HitsOriginal)
	assert.Equal(t, domain.ScoreKindRerank, sc.HitsOriginal[0].ScoreKind)
	require.NotEmpty(t, sc.HitsExpanded)
	assert.Equal(t, domain.ScoreKindRerank, sc.HitsExpanded[0].ScoreKind)
}

// TestRerank_Failure_LeavesTheRetrievalScoreSpaceIntact: a reranker outage must
// not leave hits claiming to carry cross-encoder scores.
func TestRerank_Failure_LeavesTheRetrievalScoreSpaceIntact(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID: "score-kind-rerank-fail",
		Query:       "why did this happen",
		HitsOriginal: []domain.SearchResult{
			{Chunk: domain.RagChunk{Content: "first"}, ArticleID: "art-1", Score: 12.5, ScoreKind: domain.ScoreKindBM25},
		},
	}

	retrieval.Rerank(context.Background(), sc, &failingReranker{}, retrieval.RerankConfig{
		Enabled:       true,
		TopK:          10,
		MaxCandidates: 40,
		Timeout:       time.Second,
	}, discardLogger())

	require.Len(t, sc.HitsOriginal, 1)
	assert.Equal(t, domain.ScoreKindBM25, sc.HitsOriginal[0].ScoreKind)
	assert.False(t, sc.RerankApplied)
}

// TestRetrievalGraph_Execute_EveryContextDeclaresItsScoreSpace closes the loop:
// no context may leave the pipeline with an unknown score space, or the gate is
// back to guessing from the magnitude.
func TestRetrievalGraph_Execute_EveryContextDeclaresItsScoreSpace(t *testing.T) {
	expander := new(mockQueryExpander)
	search := new(mockSearchClient)
	encoder := new(mockVectorEncoder)
	chunkRepo := new(mockChunkRepo)

	queryVec := []float32{0.1, 0.2, 0.3}
	expander.On("ExpandQuery", mock.Anything, "test query", 1, 3).Return([]string{"expanded"}, nil)
	search.On("Search", mock.Anything, "test query").Return([]domain.SearchHit{}, nil)
	encoder.On("Encode", mock.Anything, mock.Anything).Return([][]float32{queryVec}, nil)
	chunkRepo.On("Search", mock.Anything, mock.Anything, 50).Return([]domain.SearchResult{
		{
			Chunk:     domain.RagChunk{ID: uuid.New(), Content: "hit", CreatedAt: time.Now()},
			Score:     0.9,
			ScoreKind: domain.ScoreKindVector,
			ArticleID: "art-1",
			Title:     "Article",
		},
	}, nil)

	g := retrieval.NewRetrievalGraph(retrieval.GraphDeps{
		QueryExpander: expander,
		LLMClient:     new(mockLLMClient),
		SearchClient:  search,
		Encoder:       encoder,
		ChunkRepo:     chunkRepo,
		Config: retrieval.GraphConfig{
			SearchLimit:                      50,
			RRFK:                             60.0,
			QuotaOriginal:                    5,
			QuotaExpanded:                    5,
			DynamicLanguageAllocationEnabled: true,
		},
		Logger: discardLogger(),
	})

	out, err := g.Execute(context.Background(), retrieval.GraphInput{Query: "test query"})
	require.NoError(t, err)
	require.NotEmpty(t, out.Contexts)
	for _, c := range out.Contexts {
		assert.NotEqual(t, domain.ScoreKindUnknown, c.ScoreKind,
			"a context with no declared score space cannot be gated")
	}
}
