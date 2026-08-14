package retrieval_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRetrievalGraph_Execute_CandidateScoped_DoesNotLeakCorpusWideBM25Hits(t *testing.T) {
	// Morning Letter scopes retrieval to the article IDs of a 24h window.
	// SearchBM25 carries no article filter, so a corpus-wide BM25 arm fuses
	// hits from outside the window into the original-query results, and the
	// letter ends up citing a months-old article — dated year 1, because a
	// BM25-only hit has no Chunk.CreatedAt.
	expander := new(mockQueryExpander)
	search := new(mockSearchClient)
	encoder := new(mockVectorEncoder)
	chunkRepo := new(mockChunkRepo)
	bm25 := new(mockBM25Searcher)

	queryVec := []float32{0.1, 0.2, 0.3}
	articleIDs := []string{"art-in-window"}

	expander.On("ExpandQuery", mock.Anything, "today's news", 1, 3).Return([]string{}, nil)
	search.On("Search", mock.Anything, "today's news").Return([]domain.SearchHit{}, nil)
	encoder.On("Encode", mock.Anything, mock.Anything).Return([][]float32{queryVec}, nil)
	chunkRepo.On("SearchWithinArticles", mock.Anything, queryVec, articleIDs, 50).Return([]domain.SearchResult{
		{
			Chunk:           domain.RagChunk{ID: uuid.New(), Content: "fresh content", CreatedAt: time.Now()},
			Score:           0.92,
			ArticleID:       "art-in-window",
			Title:           "In Window",
			URL:             "https://example.com/in-window",
			DocumentVersion: 1,
		},
	}, nil)
	// The whole corpus answers this keyword query, including a half-year-old article.
	bm25.On("SearchBM25", mock.Anything, mock.Anything, mock.Anything).Return([]domain.BM25SearchResult{
		{ArticleID: "art-six-months-old", Content: "stale content", Title: "Stale Article", Rank: 1, Score: 30.0},
	}, nil)

	g := retrieval.NewRetrievalGraph(retrieval.GraphDeps{
		QueryExpander: expander,
		LLMClient:     new(mockLLMClient),
		SearchClient:  search,
		Encoder:       encoder,
		ChunkRepo:     chunkRepo,
		BM25Searcher:  bm25,
		Config: retrieval.GraphConfig{
			SearchLimit:                      50,
			RRFK:                             60.0,
			QuotaOriginal:                    5,
			QuotaExpanded:                    5,
			HybridSearchEnabled:              true,
			BM25Limit:                        50,
			DynamicLanguageAllocationEnabled: true,
		},
		Logger: discardLogger(),
	})

	result, err := g.Execute(context.Background(), retrieval.GraphInput{
		Query:               "today's news",
		CandidateArticleIDs: articleIDs,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Contexts, "the in-window article must still be retrieved")

	for _, c := range result.Contexts {
		assert.Contains(t, articleIDs, c.ArticleID,
			"candidate-scoped retrieval must not return an article outside the requested scope")
		assert.False(t, strings.HasPrefix(c.PublishedAt, "0001-"),
			"a context with a zero-value CreatedAt would be cited as a year-1 publication date")
	}

	// A corpus-wide BM25 query cannot contribute anything usable to a scoped
	// retrieval — every hit outside the scope has to be discarded — so it must
	// not be issued at all.
	bm25.AssertNotCalled(t, "SearchBM25", mock.Anything, mock.Anything, mock.Anything)
	assert.Zero(t, result.BM25HitCount, "no BM25 arm runs under candidate scoping")
}

func TestEmbedAndSearch_CandidateArticles_SkipsUnfilterableBM25Arm(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockEncoder := new(MockVectorEncoder)
	mockBM25 := new(MockBM25Searcher)
	mockChunkRepo := new(MockRagChunkRepository)

	queryVec := []float32{0.1, 0.2, 0.3}
	sc := &retrieval.StageContext{
		RetrievalID:         "test-scoped-bm25",
		Query:               "test query",
		OriginalEmbedding:   queryVec,
		CandidateArticleIDs: []string{"art-1"},
		SearchLimit:         50,
	}

	mockBM25.On("SearchBM25", mock.Anything, mock.Anything, mock.Anything).Return([]domain.BM25SearchResult{
		{ArticleID: "art-out-of-scope", Content: "unscoped", Title: "Unscoped", Rank: 1, Score: 9.0},
	}, nil)
	mockChunkRepo.On("SearchWithinArticles", mock.Anything, queryVec, sc.CandidateArticleIDs, 50).Return([]domain.SearchResult{
		{Chunk: domain.RagChunk{Content: "scoped result"}, Score: 0.7, ArticleID: "art-1"},
	}, nil)

	err := retrieval.EmbedAndSearch(context.Background(), sc, mockEncoder, mockBM25, nil, mockChunkRepo, true, 50, logger)
	require.NoError(t, err)

	assert.Empty(t, sc.BM25Results, "BM25 has no article filter, so it must not run under candidate scoping")
	mockBM25.AssertNotCalled(t, "SearchBM25", mock.Anything, mock.Anything, mock.Anything)
	assert.Len(t, sc.OriginalResults, 1, "the scoped vector arm must still run")
}
