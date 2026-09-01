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

// TestFuseResults_ExpandedQueriesAlsoUseTheHybridSearcher: with
// HYBRID_BM25_SOURCE=postgres the in-DB hybrid searcher replaced the lexical
// arm for the original query only, so the expanded queries — the ones that
// carry the cross-language translations, i.e. exactly the queries a lexical
// match helps most — silently ran plain vector search. The asymmetry meant
// turning the flag on removed lexical matching from most of the pipeline.
func TestFuseResults_ExpandedQueriesAlsoUseTheHybridSearcher(t *testing.T) {
	repo := new(MockRagChunkRepository)
	hybrid := new(MockHybridSearcher)

	origVec := []float32{0.1, 0.2}
	expVec := []float32{0.3, 0.4}

	sc := &retrieval.StageContext{
		RetrievalID:       "hybrid-symmetry",
		Query:             "エグザンプル社の合成炉の動き",
		OriginalEmbedding: origVec,
		OriginalResults: []domain.SearchResult{
			{Chunk: domain.RagChunk{ID: uuid.New(), Content: "original", CreatedAt: time.Now()}, Score: 0.02, ScoreKind: domain.ScoreKindRRF, ArticleID: "art-1"},
		},
		AdditionalEmbeddings: [][]float32{expVec},
		AdditionalQueries:    []string{"Example Systems synthetic reactor activity"},
		SearchLimit:          50,
		RRFK:                 60.0,
	}

	hybrid.On("HybridSearch", mock.Anything, expVec, "Example Systems synthetic reactor activity", 50).
		Return([]domain.SearchResult{
			{Chunk: domain.RagChunk{ID: uuid.New(), Content: "expanded", CreatedAt: time.Now()}, Score: 0.03, ScoreKind: domain.ScoreKindRRF, ArticleID: "art-2", Title: "EN Article"},
		}, nil)

	require.NoError(t, retrieval.FuseResults(context.Background(), sc, repo, hybrid, true, discardLogger()))

	hybrid.AssertCalled(t, "HybridSearch", mock.Anything, expVec, "Example Systems synthetic reactor activity", 50)
	repo.AssertNotCalled(t, "Search", mock.Anything, mock.Anything, mock.Anything)
	require.Len(t, sc.HitsExpanded, 1)
	assert.Equal(t, "EN Article", sc.HitsExpanded[0].Title)
	assert.Equal(t, domain.ScoreKindRRF, sc.HitsExpanded[0].ScoreKind)
}

// TestFuseResults_ExpandedQueriesFallBackToVectorUnderCandidateScope: the
// hybrid searcher has no article filter, so candidate-scoped retrieval
// (Morning Letter) must keep using the scoped chunk-repo query — the same
// carve-out Stage 2 makes for the original query.
func TestFuseResults_ExpandedQueriesFallBackToVectorUnderCandidateScope(t *testing.T) {
	repo := new(MockRagChunkRepository)
	hybrid := new(MockHybridSearcher)

	expVec := []float32{0.3, 0.4}
	articleIDs := []string{"art-in-window"}

	sc := &retrieval.StageContext{
		RetrievalID:          "hybrid-symmetry-scoped",
		Query:                "today's news",
		OriginalEmbedding:    []float32{0.1, 0.2},
		CandidateArticleIDs:  articleIDs,
		AdditionalEmbeddings: [][]float32{expVec},
		AdditionalQueries:    []string{"expanded"},
		SearchLimit:          50,
		RRFK:                 60.0,
	}

	repo.On("SearchWithinArticles", mock.Anything, expVec, articleIDs, 50).
		Return([]domain.SearchResult{
			{Chunk: domain.RagChunk{ID: uuid.New(), Content: "scoped", CreatedAt: time.Now()}, Score: 0.8, ScoreKind: domain.ScoreKindVector, ArticleID: "art-in-window"},
		}, nil)

	require.NoError(t, retrieval.FuseResults(context.Background(), sc, repo, hybrid, true, discardLogger()))

	hybrid.AssertNotCalled(t, "HybridSearch", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	require.Len(t, sc.HitsExpanded, 1)
}

// TestFuseResults_ExpandedQueriesUseVectorWhenHybridDisabled guards the
// meilisearch-source default: without an in-DB hybrid searcher the expanded
// arm stays on plain vector search.
func TestFuseResults_ExpandedQueriesUseVectorWhenHybridDisabled(t *testing.T) {
	repo := new(MockRagChunkRepository)
	hybrid := new(MockHybridSearcher)

	expVec := []float32{0.3, 0.4}
	sc := &retrieval.StageContext{
		RetrievalID:          "hybrid-symmetry-off",
		Query:                "query",
		OriginalEmbedding:    []float32{0.1, 0.2},
		AdditionalEmbeddings: [][]float32{expVec},
		AdditionalQueries:    []string{"expanded"},
		SearchLimit:          50,
		RRFK:                 60.0,
	}

	repo.On("Search", mock.Anything, expVec, 50).Return([]domain.SearchResult{
		{Chunk: domain.RagChunk{ID: uuid.New(), Content: "vector", CreatedAt: time.Now()}, Score: 0.8, ScoreKind: domain.ScoreKindVector, ArticleID: "art-1"},
	}, nil)

	require.NoError(t, retrieval.FuseResults(context.Background(), sc, repo, hybrid, false, discardLogger()))

	hybrid.AssertNotCalled(t, "HybridSearch", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertCalled(t, "Search", mock.Anything, expVec, 50)
}
