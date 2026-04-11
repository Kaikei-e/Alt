package retrieval_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockVectorEncoder is a test double for domain.VectorEncoder.
type MockVectorEncoder struct {
	mock.Mock
}

func (m *MockVectorEncoder) Encode(ctx context.Context, texts []string) ([][]float32, error) {
	args := m.Called(ctx, texts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([][]float32), args.Error(1)
}

func (m *MockVectorEncoder) Version() string { return "mock-v1" }

// MockBM25Searcher is a test double for domain.BM25Searcher.
type MockBM25Searcher struct {
	mock.Mock
}

func (m *MockBM25Searcher) SearchBM25(ctx context.Context, query string, limit int) ([]domain.BM25SearchResult, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.BM25SearchResult), args.Error(1)
}

func TestEmbedAndSearch_NilEmbedding_SkipsVectorSearch_RunsBM25(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockEncoder := new(MockVectorEncoder)
	mockBM25 := new(MockBM25Searcher)
	mockChunkRepo := new(MockRagChunkRepository)

	sc := &retrieval.StageContext{
		RetrievalID:       "test-degraded",
		Query:             "test query",
		OriginalEmbedding: nil, // Embedding failed in Stage 1
		SearchLimit:       50,
	}

	// BM25 should still be called
	mockBM25.On("SearchBM25", mock.Anything, "test query", 50).Return([]domain.BM25SearchResult{
		{ArticleID: "art-1", Content: "BM25 content", Title: "BM25 Title", Rank: 1, Score: 10.0},
	}, nil)

	// Vector search should NOT be called (embedding is nil)
	// If it is called, the mock will panic with "unexpected call"

	err := retrieval.EmbedAndSearch(context.Background(), sc, mockEncoder, mockBM25, mockChunkRepo, true, 50, logger)
	require.NoError(t, err, "EmbedAndSearch should succeed in BM25-only degraded mode")

	assert.Len(t, sc.BM25Results, 1, "BM25 results should be populated")
	assert.Empty(t, sc.OriginalResults, "vector search should not run when embedding is nil")
	mockChunkRepo.AssertNotCalled(t, "Search", mock.Anything, mock.Anything, mock.Anything)
}

func TestEmbedAndSearch_NilEmbedding_NoBM25Searcher_NoError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockEncoder := new(MockVectorEncoder)
	mockChunkRepo := new(MockRagChunkRepository)

	sc := &retrieval.StageContext{
		RetrievalID:       "test-degraded-no-bm25",
		Query:             "test query",
		OriginalEmbedding: nil,
		SearchLimit:       50,
	}

	// No BM25 searcher, no vector search possible — should complete without error
	err := retrieval.EmbedAndSearch(context.Background(), sc, mockEncoder, nil, mockChunkRepo, false, 50, logger)
	require.NoError(t, err, "EmbedAndSearch should not error even with no search available")

	assert.Empty(t, sc.OriginalResults)
	assert.Empty(t, sc.BM25Results)
}

func TestEmbedAndSearch_WithEmbedding_RunsVectorSearch(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockEncoder := new(MockVectorEncoder)
	mockChunkRepo := new(MockRagChunkRepository)

	queryVec := []float32{0.1, 0.2, 0.3}
	sc := &retrieval.StageContext{
		RetrievalID:       "test-normal",
		Query:             "test query",
		OriginalEmbedding: queryVec,
		SearchLimit:       50,
	}

	mockChunkRepo.On("Search", mock.Anything, queryVec, 50).Return([]domain.SearchResult{
		{Chunk: domain.RagChunk{Content: "result"}, Score: 0.9, Title: "Article"},
	}, nil)

	err := retrieval.EmbedAndSearch(context.Background(), sc, mockEncoder, nil, mockChunkRepo, false, 50, logger)
	require.NoError(t, err)

	assert.Len(t, sc.OriginalResults, 1)
	mockChunkRepo.AssertCalled(t, "Search", mock.Anything, queryVec, 50)
}

func TestEmbedAndSearch_BM25UsesExpandedQueries(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockEncoder := new(MockVectorEncoder)
	mockBM25 := new(MockBM25Searcher)
	mockChunkRepo := new(MockRagChunkRepository)

	queryVec := []float32{0.1, 0.2, 0.3}
	sc := &retrieval.StageContext{
		RetrievalID:       "test-bm25-expanded",
		Query:             "ヴァンス副大統領の動き",
		OriginalEmbedding: queryVec,
		ExpandedQueries:   []string{"JD Vance vice president activities"}, // English translation
		SearchLimit:       50,
	}

	// BM25 should be called for BOTH original query AND expanded query
	mockBM25.On("SearchBM25", mock.Anything, "ヴァンス副大統領の動き", 50).Return([]domain.BM25SearchResult{
		{ArticleID: "art-jp", Content: "日本語結果", Title: "JP Article", Rank: 1, Score: 5.0},
	}, nil)
	mockBM25.On("SearchBM25", mock.Anything, "JD Vance vice president activities", 50).Return([]domain.BM25SearchResult{
		{ArticleID: "art-en", Content: "English result", Title: "EN Article", Rank: 1, Score: 12.0},
	}, nil)

	// Vector search setup
	mockChunkRepo.On("Search", mock.Anything, queryVec, 50).Return([]domain.SearchResult{}, nil)
	// Expanded embedding
	mockEncoder.On("Encode", mock.Anything, mock.Anything).Return([][]float32{queryVec}, nil)

	err := retrieval.EmbedAndSearch(context.Background(), sc, mockEncoder, mockBM25, mockChunkRepo, true, 50, logger)
	require.NoError(t, err)

	// Both original and expanded query should have been searched via BM25
	mockBM25.AssertCalled(t, "SearchBM25", mock.Anything, "ヴァンス副大統領の動き", 50)
	mockBM25.AssertCalled(t, "SearchBM25", mock.Anything, "JD Vance vice president activities", 50)

	// Results should be merged (2 articles from 2 queries)
	assert.GreaterOrEqual(t, len(sc.BM25Results), 2, "BM25 results from both queries should be merged")
}

func TestEmbedAndSearch_BM25ExpandedDeduplicatesResults(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockEncoder := new(MockVectorEncoder)
	mockBM25 := new(MockBM25Searcher)
	mockChunkRepo := new(MockRagChunkRepository)

	queryVec := []float32{0.1, 0.2, 0.3}
	sc := &retrieval.StageContext{
		RetrievalID:       "test-bm25-dedup",
		Query:             "query A",
		OriginalEmbedding: queryVec,
		ExpandedQueries:   []string{"query B"},
		SearchLimit:       50,
	}

	// Both queries return the same article
	sameResult := []domain.BM25SearchResult{
		{ArticleID: "art-same", Content: "same content", Title: "Same", Rank: 1, Score: 10.0},
	}
	mockBM25.On("SearchBM25", mock.Anything, "query A", 50).Return(sameResult, nil)
	mockBM25.On("SearchBM25", mock.Anything, "query B", 50).Return(sameResult, nil)
	mockChunkRepo.On("Search", mock.Anything, queryVec, 50).Return([]domain.SearchResult{}, nil)
	mockEncoder.On("Encode", mock.Anything, mock.Anything).Return([][]float32{queryVec}, nil)

	err := retrieval.EmbedAndSearch(context.Background(), sc, mockEncoder, mockBM25, mockChunkRepo, true, 50, logger)
	require.NoError(t, err)

	// Duplicate articles should be deduplicated
	assert.Len(t, sc.BM25Results, 1, "duplicate articles from multi-query BM25 should be deduplicated")
}

func TestEmbedAndSearch_VectorSearchFails_ReturnsError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockEncoder := new(MockVectorEncoder)
	mockChunkRepo := new(MockRagChunkRepository)

	queryVec := []float32{0.1, 0.2, 0.3}
	sc := &retrieval.StageContext{
		RetrievalID:       "test-search-fail",
		Query:             "test query",
		OriginalEmbedding: queryVec,
		SearchLimit:       50,
	}

	mockChunkRepo.On("Search", mock.Anything, queryVec, 50).Return(nil, fmt.Errorf("db connection lost"))

	err := retrieval.EmbedAndSearch(context.Background(), sc, mockEncoder, nil, mockChunkRepo, false, 50, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to search original query")
}
