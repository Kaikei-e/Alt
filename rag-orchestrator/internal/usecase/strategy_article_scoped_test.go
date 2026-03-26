package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestArticleScopedStrategy_Success(t *testing.T) {
	docRepo := new(MockRagDocumentRepository)
	chunkRepo := new(MockRagChunkRepository)
	strategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, testLogger())

	ctx := context.Background()
	articleID := "test-article-123"
	versionID := uuid.New()
	chunkID1 := uuid.New()
	chunkID2 := uuid.New()

	docRepo.On("GetByArticleID", ctx, articleID).Return(&domain.RagDocument{
		ID:               uuid.New(),
		ArticleID:        articleID,
		CurrentVersionID: &versionID,
	}, nil)

	docRepo.On("GetVersionByID", ctx, versionID).Return(&domain.RagDocumentVersion{
		ID:            versionID,
		VersionNumber: 3,
		Title:         "Test Article",
		URL:           "https://example.com/article",
		CreatedAt:     time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}, nil)

	chunkRepo.On("GetChunksByVersionID", ctx, versionID).Return([]domain.RagChunk{
		{ID: chunkID1, Content: "First chunk content", Ordinal: 0},
		{ID: chunkID2, Content: "Second chunk content", Ordinal: 1},
	}, nil)

	intent := usecase.QueryIntent{
		IntentType: usecase.IntentArticleScoped,
		ArticleID:  articleID,
	}

	output, err := strategy.Retrieve(ctx, usecase.RetrieveContextInput{Query: "test"}, intent)
	assert.NoError(t, err)
	assert.Len(t, output.Contexts, 2)
	assert.Equal(t, float32(1.0), output.Contexts[0].Score)
	assert.Equal(t, "Test Article", output.Contexts[0].Title)
	assert.Equal(t, "https://example.com/article", output.Contexts[0].URL)
	assert.Equal(t, 3, output.Contexts[0].DocumentVersion)
	assert.Equal(t, chunkID1, output.Contexts[0].ChunkID)
	assert.Equal(t, "First chunk content", output.Contexts[0].ChunkText)
}

func TestArticleScopedStrategy_ArticleNotFound(t *testing.T) {
	docRepo := new(MockRagDocumentRepository)
	chunkRepo := new(MockRagChunkRepository)
	strategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, testLogger())

	ctx := context.Background()
	docRepo.On("GetByArticleID", ctx, "missing").Return(nil, nil)

	intent := usecase.QueryIntent{
		IntentType: usecase.IntentArticleScoped,
		ArticleID:  "missing",
	}
	_, err := strategy.Retrieve(ctx, usecase.RetrieveContextInput{Query: "test"}, intent)
	assert.ErrorIs(t, err, usecase.ErrArticleNotIndexed)
}

func TestArticleScopedStrategy_NoCurrentVersion(t *testing.T) {
	docRepo := new(MockRagDocumentRepository)
	chunkRepo := new(MockRagChunkRepository)
	strategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, testLogger())

	ctx := context.Background()
	docRepo.On("GetByArticleID", ctx, "no-ver").Return(&domain.RagDocument{
		ID:               uuid.New(),
		ArticleID:        "no-ver",
		CurrentVersionID: nil,
	}, nil)

	intent := usecase.QueryIntent{
		IntentType: usecase.IntentArticleScoped,
		ArticleID:  "no-ver",
	}
	_, err := strategy.Retrieve(ctx, usecase.RetrieveContextInput{Query: "test"}, intent)
	assert.ErrorIs(t, err, usecase.ErrArticleNotIndexed)
}

func TestArticleScopedStrategy_VersionMismatch(t *testing.T) {
	// When CurrentVersionID differs from latest, we should use CurrentVersionID
	docRepo := new(MockRagDocumentRepository)
	chunkRepo := new(MockRagChunkRepository)
	strategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, testLogger())

	ctx := context.Background()
	currentVersionID := uuid.New()
	chunkID := uuid.New()

	docRepo.On("GetByArticleID", ctx, "art-1").Return(&domain.RagDocument{
		ID:               uuid.New(),
		ArticleID:        "art-1",
		CurrentVersionID: &currentVersionID,
	}, nil)

	// GetVersionByID is called with currentVersionID, not latest
	docRepo.On("GetVersionByID", ctx, currentVersionID).Return(&domain.RagDocumentVersion{
		ID:            currentVersionID,
		VersionNumber: 2, // Not the latest (3), but the current
		Title:         "Current Title",
		URL:           "https://example.com/current",
		CreatedAt:     time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
	}, nil)

	chunkRepo.On("GetChunksByVersionID", ctx, currentVersionID).Return([]domain.RagChunk{
		{ID: chunkID, Content: "Chunk from current version", Ordinal: 0},
	}, nil)

	intent := usecase.QueryIntent{IntentType: usecase.IntentArticleScoped, ArticleID: "art-1"}
	output, err := strategy.Retrieve(ctx, usecase.RetrieveContextInput{Query: "test"}, intent)
	assert.NoError(t, err)
	assert.Equal(t, 2, output.Contexts[0].DocumentVersion)
	assert.Equal(t, "Current Title", output.Contexts[0].Title)

	// Verify GetLatestVersion was NOT called
	docRepo.AssertNotCalled(t, "GetLatestVersion", mock.Anything, mock.Anything)
}

func TestArticleScopedStrategy_NoChunks(t *testing.T) {
	docRepo := new(MockRagDocumentRepository)
	chunkRepo := new(MockRagChunkRepository)
	strategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, testLogger())

	ctx := context.Background()
	versionID := uuid.New()

	docRepo.On("GetByArticleID", ctx, "empty").Return(&domain.RagDocument{
		ID:               uuid.New(),
		ArticleID:        "empty",
		CurrentVersionID: &versionID,
	}, nil)
	docRepo.On("GetVersionByID", ctx, versionID).Return(&domain.RagDocumentVersion{
		ID: versionID, VersionNumber: 1, Title: "Empty", CreatedAt: time.Now(),
	}, nil)
	chunkRepo.On("GetChunksByVersionID", ctx, versionID).Return([]domain.RagChunk{}, nil)

	intent := usecase.QueryIntent{IntentType: usecase.IntentArticleScoped, ArticleID: "empty"}
	_, err := strategy.Retrieve(ctx, usecase.RetrieveContextInput{Query: "test"}, intent)
	assert.ErrorIs(t, err, usecase.ErrArticleNotIndexed)
}

func TestArticleScopedStrategy_FollowUpReranksChunksByRelevance(t *testing.T) {
	docRepo := new(MockRagDocumentRepository)
	chunkRepo := new(MockRagChunkRepository)
	strategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, testLogger())

	ctx := context.Background()
	articleID := "rerank-article"
	versionID := uuid.New()

	docRepo.On("GetByArticleID", ctx, articleID).Return(&domain.RagDocument{
		ID: uuid.New(), ArticleID: articleID, CurrentVersionID: &versionID,
	}, nil)
	docRepo.On("GetVersionByID", ctx, versionID).Return(&domain.RagDocumentVersion{
		ID: versionID, VersionNumber: 1, Title: "Test Article",
		URL: "https://example.com", CreatedAt: time.Now(),
	}, nil)

	chunkRepo.On("GetChunksByVersionID", ctx, versionID).Return([]domain.RagChunk{
		{ID: uuid.New(), Content: "Introduction and background of the topic", Ordinal: 0},
		{ID: uuid.New(), Content: "Security implications and vulnerability details", Ordinal: 1},
		{ID: uuid.New(), Content: "Performance benchmarks and latency measurements", Ordinal: 2},
		{ID: uuid.New(), Content: "Implementation architecture and system design", Ordinal: 3},
	}, nil)

	intent := usecase.QueryIntent{
		IntentType:   usecase.IntentArticleScoped,
		ArticleID:    articleID,
		UserQuestion: "What are the security implications?",
	}

	// Follow-up about security — chunks about security should rank higher
	input := usecase.RetrieveContextInput{
		Query: "What are the security implications?",
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "What is this article about?"},
			{Role: "assistant", Content: "This article covers a new protocol..."},
		},
	}

	output, err := strategy.Retrieve(ctx, input, intent)
	assert.NoError(t, err)
	assert.Len(t, output.Contexts, 4)

	// With conversation history present, chunks should be reranked by relevance.
	// Not all scores should be uniform 1.0 — BM25 should produce varying scores.
	allSameScore := true
	for i := 1; i < len(output.Contexts); i++ {
		if output.Contexts[i].Score != output.Contexts[0].Score {
			allSameScore = false
			break
		}
	}
	assert.False(t, allSameScore, "follow-up should produce varying scores, not uniform 1.0")

	// Scores should be sorted descending (most relevant first)
	for i := 1; i < len(output.Contexts); i++ {
		assert.GreaterOrEqual(t, output.Contexts[i-1].Score, output.Contexts[i].Score,
			"chunks should be sorted by relevance score descending")
	}

	// The security chunk should rank first for a security-related follow-up
	assert.Contains(t, output.Contexts[0].ChunkText, "Security",
		"security chunk should be first for security-related follow-up")
}

func TestArticleScopedStrategy_FollowUpCrossLanguageTranslates(t *testing.T) {
	docRepo := new(MockRagDocumentRepository)
	chunkRepo := new(MockRagChunkRepository)
	mockExpander := new(MockQueryExpander)
	strategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, testLogger(), mockExpander)

	ctx := context.Background()
	articleID := "cross-lang"
	versionID := uuid.New()

	docRepo.On("GetByArticleID", ctx, articleID).Return(&domain.RagDocument{
		ID: uuid.New(), ArticleID: articleID, CurrentVersionID: &versionID,
	}, nil)
	docRepo.On("GetVersionByID", ctx, versionID).Return(&domain.RagDocumentVersion{
		ID: versionID, VersionNumber: 1, Title: "English Article",
		URL: "https://example.com", CreatedAt: time.Now(),
	}, nil)
	chunkRepo.On("GetChunksByVersionID", ctx, versionID).Return([]domain.RagChunk{
		{ID: uuid.New(), Content: "The fuel crisis in Australia deepens", Ordinal: 0},
		{ID: uuid.New(), Content: "Government announces emergency measures", Ordinal: 1},
		{ID: uuid.New(), Content: "Root cause of the crisis and geopolitical tensions", Ordinal: 2},
	}, nil)

	// QueryExpander translates Japanese → English
	mockExpander.On("ExpandQueryWithHistory", mock.Anything,
		"この危機の原因はなに？", mock.Anything, 0, 1,
	).Return([]string{"root cause of crisis"}, nil)

	intent := usecase.QueryIntent{
		IntentType:   usecase.IntentArticleScoped,
		ArticleID:    articleID,
		UserQuestion: "この危機の原因はなに？",
	}
	input := usecase.RetrieveContextInput{
		Query: "この危機の原因はなに？",
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "What is this about?"},
			{Role: "assistant", Content: "This is about the fuel crisis."},
		},
	}

	output, err := strategy.Retrieve(ctx, input, intent)
	assert.NoError(t, err)
	assert.Len(t, output.Contexts, 3)

	// After translation, BM25 should match English terms and produce varying scores
	// "Root cause" chunk should score highest
	assert.Contains(t, output.Contexts[0].ChunkText, "Root cause",
		"translated query should match root-cause chunk")

	// QueryExpander should have been called with the Japanese query
	mockExpander.AssertCalled(t, "ExpandQueryWithHistory", mock.Anything,
		"この危機の原因はなに？", mock.Anything, 0, 1)
}

func TestArticleScopedStrategy_FollowUpNoExpanderFallsBack(t *testing.T) {
	docRepo := new(MockRagDocumentRepository)
	chunkRepo := new(MockRagChunkRepository)
	// No query expander — should fall back gracefully
	strategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, testLogger())

	ctx := context.Background()
	articleID := "no-expander"
	versionID := uuid.New()

	docRepo.On("GetByArticleID", ctx, articleID).Return(&domain.RagDocument{
		ID: uuid.New(), ArticleID: articleID, CurrentVersionID: &versionID,
	}, nil)
	docRepo.On("GetVersionByID", ctx, versionID).Return(&domain.RagDocumentVersion{
		ID: versionID, VersionNumber: 1, Title: "English Article",
		URL: "https://example.com", CreatedAt: time.Now(),
	}, nil)
	chunkRepo.On("GetChunksByVersionID", ctx, versionID).Return([]domain.RagChunk{
		{ID: uuid.New(), Content: "English content here", Ordinal: 0},
		{ID: uuid.New(), Content: "More English text", Ordinal: 1},
	}, nil)

	intent := usecase.QueryIntent{
		IntentType:   usecase.IntentArticleScoped,
		ArticleID:    articleID,
		UserQuestion: "日本語クエリ",
	}
	input := usecase.RetrieveContextInput{
		Query: "日本語クエリ",
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "What?"},
			{Role: "assistant", Content: "Answer"},
		},
	}

	output, err := strategy.Retrieve(ctx, input, intent)
	assert.NoError(t, err)
	// Without expander, BM25 won't match → scores fallback to 1.0
	for _, ctx := range output.Contexts {
		assert.Equal(t, float32(1.0), ctx.Score,
			"without expander, cross-language should fall back to 1.0")
	}
}

func TestArticleScopedStrategy_FirstTurnKeepsOriginalOrder(t *testing.T) {
	docRepo := new(MockRagDocumentRepository)
	chunkRepo := new(MockRagChunkRepository)
	strategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, testLogger())

	ctx := context.Background()
	articleID := "first-turn"
	versionID := uuid.New()

	docRepo.On("GetByArticleID", ctx, articleID).Return(&domain.RagDocument{
		ID: uuid.New(), ArticleID: articleID, CurrentVersionID: &versionID,
	}, nil)
	docRepo.On("GetVersionByID", ctx, versionID).Return(&domain.RagDocumentVersion{
		ID: versionID, VersionNumber: 1, Title: "Test",
		URL: "https://example.com", CreatedAt: time.Now(),
	}, nil)
	chunkRepo.On("GetChunksByVersionID", ctx, versionID).Return([]domain.RagChunk{
		{ID: uuid.New(), Content: "Chunk A", Ordinal: 0},
		{ID: uuid.New(), Content: "Chunk B", Ordinal: 1},
	}, nil)

	intent := usecase.QueryIntent{IntentType: usecase.IntentArticleScoped, ArticleID: articleID}

	// First turn: no conversation history → original ordinal order, score 1.0
	input := usecase.RetrieveContextInput{
		Query:               "What is this about?",
		ConversationHistory: nil,
	}
	output, err := strategy.Retrieve(ctx, input, intent)
	assert.NoError(t, err)
	assert.Equal(t, float32(1.0), output.Contexts[0].Score)
	assert.Equal(t, "Chunk A", output.Contexts[0].ChunkText)
	assert.Equal(t, "Chunk B", output.Contexts[1].ChunkText)
}

func TestArticleScopedStrategy_ChunkOrdering(t *testing.T) {
	docRepo := new(MockRagDocumentRepository)
	chunkRepo := new(MockRagChunkRepository)
	strategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, testLogger())

	ctx := context.Background()
	versionID := uuid.New()

	docRepo.On("GetByArticleID", ctx, "ordered").Return(&domain.RagDocument{
		ID: uuid.New(), ArticleID: "ordered", CurrentVersionID: &versionID,
	}, nil)
	docRepo.On("GetVersionByID", ctx, versionID).Return(&domain.RagDocumentVersion{
		ID: versionID, VersionNumber: 1, Title: "Ordered", CreatedAt: time.Now(),
	}, nil)

	// GetChunksByVersionID returns chunks ordered by ordinal ASC (DB guarantee)
	chunkRepo.On("GetChunksByVersionID", ctx, versionID).Return([]domain.RagChunk{
		{ID: uuid.New(), Content: "Chunk 0", Ordinal: 0},
		{ID: uuid.New(), Content: "Chunk 1", Ordinal: 1},
		{ID: uuid.New(), Content: "Chunk 2", Ordinal: 2},
	}, nil)

	intent := usecase.QueryIntent{IntentType: usecase.IntentArticleScoped, ArticleID: "ordered"}
	output, err := strategy.Retrieve(ctx, usecase.RetrieveContextInput{Query: "test"}, intent)
	assert.NoError(t, err)
	assert.Len(t, output.Contexts, 3)
	assert.Equal(t, "Chunk 0", output.Contexts[0].ChunkText)
	assert.Equal(t, "Chunk 1", output.Contexts[1].ChunkText)
	assert.Equal(t, "Chunk 2", output.Contexts[2].ChunkText)
}
