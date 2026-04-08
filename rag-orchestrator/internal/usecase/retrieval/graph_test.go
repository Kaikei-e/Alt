package retrieval_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mocks ---

type mockQueryExpander struct{ mock.Mock }

func (m *mockQueryExpander) ExpandQuery(ctx context.Context, query string, japaneseCount, englishCount int) ([]string, error) {
	args := m.Called(ctx, query, japaneseCount, englishCount)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockQueryExpander) ExpandQueryWithHistory(ctx context.Context, query string, history []domain.Message, japaneseCount, englishCount int) ([]string, error) {
	args := m.Called(ctx, query, history, japaneseCount, englishCount)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

type mockLLMClient struct{ mock.Mock }

func (m *mockLLMClient) Generate(ctx context.Context, prompt string, maxTokens int) (*domain.LLMResponse, error) {
	args := m.Called(ctx, prompt, maxTokens)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.LLMResponse), args.Error(1)
}

func (m *mockLLMClient) GenerateStream(ctx context.Context, prompt string, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	return nil, nil, nil
}

func (m *mockLLMClient) Chat(ctx context.Context, messages []domain.Message, maxTokens int) (*domain.LLMResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) ChatStream(ctx context.Context, messages []domain.Message, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	return nil, nil, nil
}

func (m *mockLLMClient) Version() string { return "mock-v1" }

type mockSearchClient struct{ mock.Mock }

func (m *mockSearchClient) Search(ctx context.Context, query string) ([]domain.SearchHit, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.SearchHit), args.Error(1)
}

type mockVectorEncoder struct{ mock.Mock }

func (m *mockVectorEncoder) Encode(ctx context.Context, texts []string) ([][]float32, error) {
	args := m.Called(ctx, texts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([][]float32), args.Error(1)
}

func (m *mockVectorEncoder) Version() string { return "mock-v1" }

type mockChunkRepo struct{ mock.Mock }

func (m *mockChunkRepo) BulkInsertChunks(ctx context.Context, chunks []domain.RagChunk) error {
	return nil
}

func (m *mockChunkRepo) GetChunksByVersionID(ctx context.Context, versionID uuid.UUID) ([]domain.RagChunk, error) {
	return nil, nil
}

func (m *mockChunkRepo) InsertEvents(ctx context.Context, events []domain.RagChunkEvent) error {
	return nil
}

func (m *mockChunkRepo) Search(ctx context.Context, queryVector []float32, limit int) ([]domain.SearchResult, error) {
	args := m.Called(ctx, queryVector, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.SearchResult), args.Error(1)
}

func (m *mockChunkRepo) SearchWithinArticles(ctx context.Context, queryVector []float32, articleIDs []string, limit int) ([]domain.SearchResult, error) {
	args := m.Called(ctx, queryVector, articleIDs, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.SearchResult), args.Error(1)
}

type mockReranker struct{ mock.Mock }

func (m *mockReranker) Rerank(ctx context.Context, query string, candidates []domain.RerankCandidate) ([]domain.RerankResult, error) {
	args := m.Called(ctx, query, candidates)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.RerankResult), args.Error(1)
}

func (m *mockReranker) ModelName() string { return "mock-reranker" }

type mockBM25Searcher struct{ mock.Mock }

func (m *mockBM25Searcher) SearchBM25(ctx context.Context, query string, limit int) ([]domain.BM25SearchResult, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.BM25SearchResult), args.Error(1)
}

// --- Tests ---

func TestNewRetrievalGraph_ReturnsNonNil(t *testing.T) {
	g := retrieval.NewRetrievalGraph(retrieval.GraphDeps{
		QueryExpander: new(mockQueryExpander),
		LLMClient:     new(mockLLMClient),
		SearchClient:  new(mockSearchClient),
		Encoder:       new(mockVectorEncoder),
		ChunkRepo:     new(mockChunkRepo),
		Logger:        discardLogger(),
	})
	assert.NotNil(t, g)
}

func TestRetrievalGraph_Execute_FullPipeline(t *testing.T) {
	// Arrange: set up mocks for all 5 stages
	expander := new(mockQueryExpander)
	llm := new(mockLLMClient)
	search := new(mockSearchClient)
	encoder := new(mockVectorEncoder)
	chunkRepo := new(mockChunkRepo)

	queryVec := []float32{0.1, 0.2, 0.3}
	chunkID := uuid.New()

	// Stage 1: ExpandQueries
	expander.On("ExpandQuery", mock.Anything, "test query", 1, 3).Return([]string{"expanded 1"}, nil)
	search.On("Search", mock.Anything, "test query").Return([]domain.SearchHit{}, nil)
	encoder.On("Encode", mock.Anything, []string{"test query"}).Return([][]float32{queryVec}, nil)

	// Stage 2: EmbedAndSearch
	encoder.On("Encode", mock.Anything, mock.MatchedBy(func(texts []string) bool {
		return len(texts) > 0 && texts[0] != "test query"
	})).Return([][]float32{queryVec}, nil)
	chunkRepo.On("Search", mock.Anything, queryVec, 50).Return([]domain.SearchResult{
		{
			Chunk:           domain.RagChunk{ID: chunkID, Content: "chunk content", CreatedAt: time.Now()},
			Score:           0.90,
			ArticleID:       "art-1",
			Title:           "Test Article",
			URL:             "https://example.com/1",
			DocumentVersion: 1,
		},
	}, nil)

	g := retrieval.NewRetrievalGraph(retrieval.GraphDeps{
		QueryExpander: expander,
		LLMClient:     llm,
		SearchClient:  search,
		Encoder:       encoder,
		ChunkRepo:     chunkRepo,
		Config: retrieval.GraphConfig{
			SearchLimit:                      50,
			RRFK:                             60.0,
			QuotaOriginal:                    5,
			QuotaExpanded:                    5,
			RerankEnabled:                    false,
			HybridSearchEnabled:              false,
			DynamicLanguageAllocationEnabled: true,
		},
		Logger: discardLogger(),
	})

	// Act
	result, err := g.Execute(context.Background(), retrieval.GraphInput{
		Query: "test query",
	})

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Contexts)
	assert.Equal(t, "chunk content", result.Contexts[0].ChunkText)
	assert.Contains(t, result.ExpandedQueries, "expanded 1")
}

func TestRetrievalGraph_Execute_EmptyQuery_ReturnsError(t *testing.T) {
	g := retrieval.NewRetrievalGraph(retrieval.GraphDeps{
		QueryExpander: new(mockQueryExpander),
		LLMClient:     new(mockLLMClient),
		SearchClient:  new(mockSearchClient),
		Encoder:       new(mockVectorEncoder),
		ChunkRepo:     new(mockChunkRepo),
		Logger:        discardLogger(),
	})

	result, err := g.Execute(context.Background(), retrieval.GraphInput{
		Query: "",
	})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "query is empty")
}

func TestRetrievalGraph_Execute_Stage1Error_PropagatesError(t *testing.T) {
	// When the original embedding fails (fatal error in Stage 1), Execute returns an error.
	encoder := new(mockVectorEncoder)
	expander := new(mockQueryExpander)
	search := new(mockSearchClient)

	expander.On("ExpandQuery", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]string{}, nil)
	search.On("Search", mock.Anything, mock.Anything).Return([]domain.SearchHit{}, nil)
	encoder.On("Encode", mock.Anything, mock.Anything).Return(nil, errors.New("encoder unavailable"))

	g := retrieval.NewRetrievalGraph(retrieval.GraphDeps{
		QueryExpander: expander,
		LLMClient:     new(mockLLMClient),
		SearchClient:  search,
		Encoder:       encoder,
		ChunkRepo:     new(mockChunkRepo),
		Logger:        discardLogger(),
	})

	result, err := g.Execute(context.Background(), retrieval.GraphInput{
		Query: "test query",
	})

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestRetrievalGraph_Execute_WithReranker(t *testing.T) {
	// Verifies that the reranker is wired in when provided.
	expander := new(mockQueryExpander)
	llm := new(mockLLMClient)
	search := new(mockSearchClient)
	encoder := new(mockVectorEncoder)
	chunkRepo := new(mockChunkRepo)
	reranker := new(mockReranker)

	queryVec := []float32{0.1, 0.2, 0.3}
	chunkID := uuid.New()

	expander.On("ExpandQuery", mock.Anything, "rerank query", 1, 3).Return([]string{}, nil)
	search.On("Search", mock.Anything, "rerank query").Return([]domain.SearchHit{}, nil)
	encoder.On("Encode", mock.Anything, mock.Anything).Return([][]float32{queryVec}, nil)
	chunkRepo.On("Search", mock.Anything, queryVec, 50).Return([]domain.SearchResult{
		{
			Chunk:           domain.RagChunk{ID: chunkID, Content: "reranked chunk", CreatedAt: time.Now()},
			Score:           0.80,
			ArticleID:       "art-2",
			Title:           "Reranked Article",
			URL:             "https://example.com/2",
			DocumentVersion: 1,
		},
	}, nil)
	reranker.On("Rerank", mock.Anything, "rerank query", mock.Anything).Return([]domain.RerankResult{
		{ID: chunkID.String(), Score: 0.99},
	}, nil)

	g := retrieval.NewRetrievalGraph(retrieval.GraphDeps{
		QueryExpander: expander,
		LLMClient:     llm,
		SearchClient:  search,
		Encoder:       encoder,
		ChunkRepo:     chunkRepo,
		Reranker:      reranker,
		Config: retrieval.GraphConfig{
			SearchLimit:                      50,
			RRFK:                             60.0,
			QuotaOriginal:                    5,
			QuotaExpanded:                    5,
			RerankEnabled:                    true,
			RerankTopK:                       10,
			RerankTimeout:                    30 * time.Second,
			HybridSearchEnabled:              false,
			DynamicLanguageAllocationEnabled: true,
		},
		Logger: discardLogger(),
	})

	result, err := g.Execute(context.Background(), retrieval.GraphInput{
		Query: "rerank query",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Contexts)
	reranker.AssertCalled(t, "Rerank", mock.Anything, "rerank query", mock.Anything)
}

func TestRetrievalGraph_Execute_WithBM25(t *testing.T) {
	// Verifies that the BM25 searcher is wired in when provided and hybrid enabled.
	expander := new(mockQueryExpander)
	search := new(mockSearchClient)
	encoder := new(mockVectorEncoder)
	chunkRepo := new(mockChunkRepo)
	bm25 := new(mockBM25Searcher)

	queryVec := []float32{0.1, 0.2, 0.3}
	chunkID := uuid.New()

	expander.On("ExpandQuery", mock.Anything, "hybrid query", 1, 3).Return([]string{}, nil)
	search.On("Search", mock.Anything, "hybrid query").Return([]domain.SearchHit{}, nil)
	encoder.On("Encode", mock.Anything, mock.Anything).Return([][]float32{queryVec}, nil)
	chunkRepo.On("Search", mock.Anything, queryVec, 50).Return([]domain.SearchResult{
		{
			Chunk:           domain.RagChunk{ID: chunkID, Content: "hybrid chunk", CreatedAt: time.Now()},
			Score:           0.85,
			ArticleID:       "art-3",
			Title:           "Hybrid Article",
			URL:             "https://example.com/3",
			DocumentVersion: 1,
		},
	}, nil)
	bm25.On("SearchBM25", mock.Anything, "hybrid query", 50).Return([]domain.BM25SearchResult{
		{ArticleID: "art-3", Content: "hybrid chunk", Title: "Hybrid Article", Rank: 1, Score: 0.70},
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
			RerankEnabled:                    false,
			HybridSearchEnabled:              true,
			BM25Limit:                        50,
			DynamicLanguageAllocationEnabled: true,
		},
		Logger: discardLogger(),
	})

	result, err := g.Execute(context.Background(), retrieval.GraphInput{
		Query: "hybrid query",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Contexts)
	assert.Greater(t, result.BM25HitCount, 0)
	bm25.AssertCalled(t, "SearchBM25", mock.Anything, "hybrid query", 50)
}

func TestRetrievalGraph_Execute_WithConversationHistory(t *testing.T) {
	// Verifies multi-turn support passes conversation history to expansion.
	expander := new(mockQueryExpander)
	search := new(mockSearchClient)
	encoder := new(mockVectorEncoder)
	chunkRepo := new(mockChunkRepo)

	queryVec := []float32{0.1, 0.2, 0.3}

	history := []domain.Message{
		{Role: "user", Content: "What is AI?"},
		{Role: "assistant", Content: "AI is artificial intelligence."},
	}

	expander.On("ExpandQueryWithHistory", mock.Anything, "tell me more", history, 1, 3).Return([]string{"AI details"}, nil)
	search.On("Search", mock.Anything, "tell me more").Return([]domain.SearchHit{}, nil)
	encoder.On("Encode", mock.Anything, []string{"tell me more"}).Return([][]float32{queryVec}, nil)
	encoder.On("Encode", mock.Anything, mock.MatchedBy(func(texts []string) bool {
		return len(texts) > 0 && texts[0] != "tell me more"
	})).Return([][]float32{queryVec}, nil)
	chunkRepo.On("Search", mock.Anything, queryVec, 50).Return([]domain.SearchResult{
		{
			Chunk:           domain.RagChunk{ID: uuid.New(), Content: "AI expanded content", CreatedAt: time.Now()},
			Score:           0.88,
			ArticleID:       "art-4",
			Title:           "AI Details",
			URL:             "https://example.com/4",
			DocumentVersion: 1,
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

	result, err := g.Execute(context.Background(), retrieval.GraphInput{
		Query:               "tell me more",
		ConversationHistory: history,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	expander.AssertCalled(t, "ExpandQueryWithHistory", mock.Anything, "tell me more", history, 1, 3)
}

func TestRetrievalGraph_Execute_WithCandidateArticleIDs(t *testing.T) {
	// Morning Letter use case: search within specific articles.
	expander := new(mockQueryExpander)
	search := new(mockSearchClient)
	encoder := new(mockVectorEncoder)
	chunkRepo := new(mockChunkRepo)

	queryVec := []float32{0.1, 0.2, 0.3}
	articleIDs := []string{"art-10", "art-11"}

	expander.On("ExpandQuery", mock.Anything, "scoped query", 1, 3).Return([]string{}, nil)
	search.On("Search", mock.Anything, "scoped query").Return([]domain.SearchHit{}, nil)
	encoder.On("Encode", mock.Anything, mock.Anything).Return([][]float32{queryVec}, nil)
	chunkRepo.On("SearchWithinArticles", mock.Anything, queryVec, articleIDs, 50).Return([]domain.SearchResult{
		{
			Chunk:           domain.RagChunk{ID: uuid.New(), Content: "scoped content", CreatedAt: time.Now()},
			Score:           0.92,
			ArticleID:       "art-10",
			Title:           "Scoped Article",
			URL:             "https://example.com/scoped",
			DocumentVersion: 1,
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

	result, err := g.Execute(context.Background(), retrieval.GraphInput{
		Query:               "scoped query",
		CandidateArticleIDs: articleIDs,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Contexts)
	chunkRepo.AssertCalled(t, "SearchWithinArticles", mock.Anything, queryVec, articleIDs, 50)
}
