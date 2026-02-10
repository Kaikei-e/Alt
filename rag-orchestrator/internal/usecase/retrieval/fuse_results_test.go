package retrieval_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockRagChunkRepository is a test double for domain.RagChunkRepository.
type MockRagChunkRepository struct {
	mock.Mock
}

func (m *MockRagChunkRepository) BulkInsertChunks(ctx context.Context, chunks []domain.RagChunk) error {
	args := m.Called(ctx, chunks)
	return args.Error(0)
}

func (m *MockRagChunkRepository) GetChunksByVersionID(ctx context.Context, versionID uuid.UUID) ([]domain.RagChunk, error) {
	args := m.Called(ctx, versionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.RagChunk), args.Error(1)
}

func (m *MockRagChunkRepository) InsertEvents(ctx context.Context, events []domain.RagChunkEvent) error {
	args := m.Called(ctx, events)
	return args.Error(0)
}

func (m *MockRagChunkRepository) Search(ctx context.Context, queryVector []float32, limit int) ([]domain.SearchResult, error) {
	args := m.Called(ctx, queryVector, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.SearchResult), args.Error(1)
}

func (m *MockRagChunkRepository) SearchWithinArticles(ctx context.Context, queryVector []float32, articleIDs []string, limit int) ([]domain.SearchResult, error) {
	args := m.Called(ctx, queryVector, articleIDs, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.SearchResult), args.Error(1)
}

func TestFuseResults_SingleQuery_NoExpansion(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockRepo := new(MockRagChunkRepository)

	chunkID := uuid.New()
	sc := &retrieval.StageContext{
		RetrievalID:       "test-fuse-1",
		Query:             "test query",
		OriginalEmbedding: []float32{0.1, 0.2},
		OriginalResults: []domain.SearchResult{
			{
				Chunk:     domain.RagChunk{ID: chunkID, Content: "original result", CreatedAt: time.Now()},
				Score:     0.95,
				Title:     "Original Article",
				ArticleID: "art-1",
			},
		},
		AdditionalEmbeddings: nil,
		AdditionalQueries:    nil,
		SearchLimit:          50,
		RRFK:                 60.0,
	}

	err := retrieval.FuseResults(context.Background(), sc, mockRepo, logger)
	require.NoError(t, err)

	assert.Len(t, sc.HitsOriginal, 1)
	assert.Equal(t, "Original Article", sc.HitsOriginal[0].Title)
	assert.Empty(t, sc.HitsExpanded, "no expanded queries means no expanded hits")
}

func TestFuseResults_WithExpandedQueries(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockRepo := new(MockRagChunkRepository)

	originalChunkID := uuid.New()
	expandedChunkID := uuid.New()

	sc := &retrieval.StageContext{
		RetrievalID:       "test-fuse-2",
		Query:             "test query",
		OriginalEmbedding: []float32{0.1, 0.2},
		OriginalResults: []domain.SearchResult{
			{
				Chunk:     domain.RagChunk{ID: originalChunkID, Content: "original", CreatedAt: time.Now()},
				Score:     0.90,
				Title:     "Original",
				ArticleID: "art-1",
			},
		},
		AdditionalEmbeddings: [][]float32{{0.3, 0.4}},
		AdditionalQueries:    []string{"expanded query 1"},
		SearchLimit:          50,
		RRFK:                 60.0,
	}

	// Mock expanded query vector search
	mockRepo.On("Search", mock.Anything, []float32{0.3, 0.4}, 50).Return([]domain.SearchResult{
		{
			Chunk:           domain.RagChunk{ID: expandedChunkID, Content: "expanded result", CreatedAt: time.Now()},
			Score:           0.85,
			Title:           "Expanded Article",
			DocumentVersion: 1,
			ArticleID:       "art-2",
		},
	}, nil)

	err := retrieval.FuseResults(context.Background(), sc, mockRepo, logger)
	require.NoError(t, err)

	assert.Len(t, sc.HitsOriginal, 1)
	assert.Len(t, sc.HitsExpanded, 1)
	assert.Equal(t, "Expanded Article", sc.HitsExpanded[0].Title)
}

func TestFuseResults_WithBM25Fusion(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockRepo := new(MockRagChunkRepository)

	chunkID := uuid.New()
	sc := &retrieval.StageContext{
		RetrievalID:       "test-fuse-3",
		Query:             "test query",
		OriginalEmbedding: []float32{0.1, 0.2},
		OriginalResults: []domain.SearchResult{
			{
				Chunk:     domain.RagChunk{ID: chunkID, Content: "result", CreatedAt: time.Now()},
				Score:     0.90,
				Title:     "Article",
				ArticleID: "art-1",
			},
		},
		BM25Results: []domain.BM25SearchResult{
			{ArticleID: "art-1", Rank: 1, Score: 10.5},
		},
		AdditionalEmbeddings: nil,
		SearchLimit:          50,
		RRFK:                 60.0,
	}

	err := retrieval.FuseResults(context.Background(), sc, mockRepo, logger)
	require.NoError(t, err)

	// After BM25 fusion, original results should have fused scores
	assert.Len(t, sc.HitsOriginal, 1)
	// Score should be RRF-based (not original vector score)
	assert.True(t, sc.HitsOriginal[0].Score > 0)
}

func TestFuseResults_SearchError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockRepo := new(MockRagChunkRepository)

	sc := &retrieval.StageContext{
		RetrievalID:          "test-fuse-4",
		Query:                "test query",
		OriginalEmbedding:    []float32{0.1, 0.2},
		OriginalResults:      nil,
		AdditionalEmbeddings: [][]float32{{0.3, 0.4}},
		AdditionalQueries:    []string{"expanded"},
		SearchLimit:          50,
		RRFK:                 60.0,
	}

	mockRepo.On("Search", mock.Anything, mock.Anything, 50).Return(nil, assert.AnError)

	err := retrieval.FuseResults(context.Background(), sc, mockRepo, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to search chunks")
}

func TestFuseResults_DeduplicatesExpandedHits(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mockRepo := new(MockRagChunkRepository)

	sharedChunkID := uuid.New()

	sc := &retrieval.StageContext{
		RetrievalID:       "test-fuse-5",
		Query:             "test query",
		OriginalEmbedding: []float32{0.1, 0.2},
		OriginalResults:   nil,
		AdditionalEmbeddings: [][]float32{
			{0.3, 0.4},
			{0.5, 0.6},
		},
		AdditionalQueries: []string{"expanded 1", "expanded 2"},
		SearchLimit:       50,
		RRFK:              60.0,
	}

	// Both expanded queries return the same chunk
	sharedResult := []domain.SearchResult{
		{
			Chunk:     domain.RagChunk{ID: sharedChunkID, Content: "shared", CreatedAt: time.Now()},
			Score:     0.85,
			Title:     "Shared Article",
			ArticleID: "art-1",
		},
	}
	mockRepo.On("Search", mock.Anything, mock.Anything, 50).Return(sharedResult, nil)

	err := retrieval.FuseResults(context.Background(), sc, mockRepo, logger)
	require.NoError(t, err)

	// Should be deduplicated to 1 entry with boosted RRF score
	assert.Len(t, sc.HitsExpanded, 1, "duplicate chunks should be merged")
}
