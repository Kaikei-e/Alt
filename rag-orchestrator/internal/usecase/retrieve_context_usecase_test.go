package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockVectorEncoder
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

func (m *MockVectorEncoder) Version() string {
	return "mock-v1"
}

// MockQueryExpander
type MockQueryExpander struct {
	mock.Mock
}

func (m *MockQueryExpander) ExpandQuery(ctx context.Context, query string, japaneseCount, englishCount int) ([]string, error) {
	args := m.Called(ctx, query, japaneseCount, englishCount)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func TestRetrieveContext_Execute_Success(t *testing.T) {
	mockChunkRepo := new(MockRagChunkRepository)
	mockDocRepo := new(MockRagDocumentRepository)
	mockEncoder := new(MockVectorEncoder)
	mockLLM := new(mockLLMClient) // Defined in answer_with_rag_usecase_test.go
	mockQueryExpander := new(MockQueryExpander)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewRetrieveContextUsecase(mockChunkRepo, mockDocRepo, mockEncoder, mockLLM, nil, mockQueryExpander, usecase.DefaultRetrievalConfig(), testLogger)

	ctx := context.Background()
	input := usecase.RetrieveContextInput{
		Query: "search query",
	}

	// Expectations
	// 1. Expand Query using QueryExpander (not LLM)
	// QueryExpander returns 4 variations (1 Japanese + 3 English)
	mockQueryExpander.On("ExpandQuery", ctx, "search query", 1, 3).Return([]string{
		"検索クエリ",
		"variation 1",
		"variation 2",
		"variation 3",
	}, nil)

	// 2. Encode
	// Expect original + 4 variations = 5 queries
	expectedQueries := []string{"search query", "検索クエリ", "variation 1", "variation 2", "variation 3"}
	// We need multiple embeddings
	queryVec := []float32{0.1, 0.2, 0.3}
	mockEncoder.On("Encode", ctx, expectedQueries).Return([][]float32{queryVec, queryVec, queryVec, queryVec, queryVec}, nil)

	// 3. Search (parallel, but mock any call)
	// Expect 5 searches (original + 4 expanded)
	// Since CandidateArticleIDs is empty, Search() is called (Augur use case)
	mockChunkRepo.On("Search", ctx, queryVec, 50).Return([]domain.SearchResult{
		{
			Chunk: domain.RagChunk{
				ID:      uuid.New(),
				Content: "Found content",
			},
			Score:           0.95,
			ArticleID:       "art-1",
			DocumentVersion: 1,
		},
	}, nil)

	// Execute
	output, err := uc.Execute(ctx, input)

	// Assert
	assert.NoError(t, err)
	// We might get duplicates if search returns same chunk, but we deduplicate in code
	// Since we return same chunk for all 5 searches, we expect 1 unique context
	assert.Len(t, output.Contexts, 1)
	assert.Equal(t, "Found content", output.Contexts[0].ChunkText)
	assert.Equal(t, float32(0.95), output.Contexts[0].Score)
}

func TestSelectContexts_DynamicAllocation(t *testing.T) {
	tests := []struct {
		name           string
		hitsOriginal   []testSearchResult
		hitsExpanded   []testContextItem
		totalQuota     int
		expectedTitles []string
	}{
		{
			name: "selects top N by score regardless of language",
			hitsOriginal: []testSearchResult{
				{title: "EN Article 1", score: 0.90, content: "content1"},
				{title: "日本語記事1", score: 0.85, content: "content2"},
			},
			hitsExpanded: []testContextItem{
				{title: "日本語記事2", score: 0.95}, // highest
				{title: "EN Article 2", score: 0.80},
				{title: "日本語記事3", score: 0.75},
			},
			totalQuota: 5,
			expectedTitles: []string{
				"日本語記事2",       // 0.95
				"EN Article 1", // 0.90
				"日本語記事1",       // 0.85
				"EN Article 2", // 0.80
				"日本語記事3",       // 0.75
			},
		},
		{
			name: "all Japanese when JA scores higher",
			hitsOriginal: []testSearchResult{
				{title: "日本語記事1", score: 0.99, content: "content1"},
				{title: "日本語記事2", score: 0.98, content: "content2"},
			},
			hitsExpanded: []testContextItem{
				{title: "日本語記事3", score: 0.97},
				{title: "EN Article", score: 0.50},
			},
			totalQuota:     3,
			expectedTitles: []string{"日本語記事1", "日本語記事2", "日本語記事3"},
		},
		{
			name: "all English when EN scores higher",
			hitsOriginal: []testSearchResult{
				{title: "EN Article 1", score: 0.99, content: "content1"},
			},
			hitsExpanded: []testContextItem{
				{title: "EN Article 2", score: 0.98},
				{title: "EN Article 3", score: 0.97},
				{title: "日本語記事", score: 0.40},
			},
			totalQuota:     3,
			expectedTitles: []string{"EN Article 1", "EN Article 2", "EN Article 3"},
		},
		{
			name: "deduplicates by ChunkID",
			hitsOriginal: []testSearchResult{
				{id: "11111111-1111-1111-1111-111111111111", title: "Same Article", score: 0.90, content: "content"},
			},
			hitsExpanded: []testContextItem{
				{id: "11111111-1111-1111-1111-111111111111", title: "Same Article", score: 0.85}, // duplicate, should be ignored
				{title: "Another Article", score: 0.80},
			},
			totalQuota:     2,
			expectedTitles: []string{"Same Article", "Another Article"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert test data to actual types
			hitsOriginal := make([]domain.SearchResult, len(tt.hitsOriginal))
			for i, hit := range tt.hitsOriginal {
				id := uuid.New()
				if hit.id != "" {
					id = uuid.MustParse(hit.id)
				}
				hitsOriginal[i] = domain.SearchResult{
					Chunk: domain.RagChunk{
						ID:      id,
						Content: hit.content,
					},
					Score: hit.score,
					Title: hit.title,
				}
			}

			hitsExpanded := make([]usecase.ContextItem, len(tt.hitsExpanded))
			for i, hit := range tt.hitsExpanded {
				id := uuid.New()
				if hit.id != "" {
					id = uuid.MustParse(hit.id)
				}
				hitsExpanded[i] = usecase.ContextItem{
					ChunkID: id,
					Title:   hit.title,
					Score:   hit.score,
				}
			}

			// Call the selection function
			contexts := usecase.SelectContextsDynamic(hitsOriginal, hitsExpanded, tt.totalQuota)

			// Verify results
			assert.Len(t, contexts, len(tt.expectedTitles), "wrong number of contexts")
			for i, expectedTitle := range tt.expectedTitles {
				assert.Equal(t, expectedTitle, contexts[i].Title, "wrong title at index %d", i)
			}
		})
	}
}

// Test helper types
type testSearchResult struct {
	id      string
	title   string
	score   float32
	content string
}

type testContextItem struct {
	id    string
	title string
	score float32
}
