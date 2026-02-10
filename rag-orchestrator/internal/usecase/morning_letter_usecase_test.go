package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockArticleClient mocks the ArticleClient interface
type MockArticleClient struct {
	mock.Mock
}

func (m *MockArticleClient) GetRecentArticles(ctx context.Context, withinHours int, limit int) ([]domain.ArticleMetadata, error) {
	args := m.Called(ctx, withinHours, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ArticleMetadata), args.Error(1)
}

// MockMorningLetterPromptBuilder mocks the MorningLetterPromptBuilder
type MockMorningLetterPromptBuilder struct {
	mock.Mock
}

func (m *MockMorningLetterPromptBuilder) Build(input usecase.MorningLetterPromptInput) ([]domain.Message, error) {
	args := m.Called(input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Message), args.Error(1)
}

func TestMorningLetterUsecase_Execute_Success(t *testing.T) {
	mockArticleClient := new(MockArticleClient)
	mockRetrieveUC := new(mockRetrieveContextUsecase) // Reuse from answer_with_rag_usecase_test.go
	mockPromptBuilder := new(MockMorningLetterPromptBuilder)
	mockLLM := new(mockLLMClient) // Reuse from answer_with_rag_usecase_test.go
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewMorningLetterUsecase(
		mockArticleClient,
		mockRetrieveUC,
		mockPromptBuilder,
		mockLLM,
		4096,
		6000,
		usecase.DefaultTemporalBoostConfig(),
		testLogger,
	)

	ctx := context.Background()
	input := usecase.MorningLetterInput{
		Query:       "What are the important news?",
		WithinHours: 24,
		TopicLimit:  5,
		Locale:      "ja",
	}

	articleID := uuid.New()
	now := time.Now()

	// Mock GetRecentArticles (limit=0 means no limit, relying on time constraint only)
	mockArticleClient.On("GetRecentArticles", ctx, 24, 0).Return([]domain.ArticleMetadata{
		{
			ID:          articleID,
			Title:       "Test Article",
			URL:         "https://example.com/article",
			PublishedAt: now.Add(-2 * time.Hour),
			FeedID:      uuid.New(),
			Tags:        []string{"tech"},
		},
	}, nil)

	// Mock RetrieveContext
	mockRetrieveUC.On("Execute", ctx, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "What are the important news?" &&
			len(input.CandidateArticleIDs) == 1 &&
			input.CandidateArticleIDs[0] == articleID.String()
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkText:   "Test article content about tech news.",
				URL:         "https://example.com/article",
				Title:       "Test Article",
				PublishedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
				Score:       0.95,
			},
		},
	}, nil)

	// Mock PromptBuilder
	mockPromptBuilder.On("Build", mock.AnythingOfType("usecase.MorningLetterPromptInput")).Return([]domain.Message{
		{Role: "system", Content: "You are a news analyst..."},
		{Role: "user", Content: "Analyze these articles..."},
	}, nil)

	// Mock LLM Chat
	mockLLM.On("Chat", ctx, mock.AnythingOfType("[]domain.Message"), 4096).Return(&domain.LLMResponse{
		Text: `{
			"topics": [
				{
					"topic": "Tech News",
					"headline": "Major tech announcement",
					"summary": "A significant tech development was announced today.",
					"importance": 0.9,
					"article_refs": [],
					"keywords": ["tech", "announcement"]
				}
			],
			"meta": {
				"topics_found": 1,
				"coverage_assessment": "comprehensive"
			}
		}`,
		Done: true,
	}, nil)

	output, err := uc.Execute(ctx, input)
	require.NoError(t, err)
	assert.NotNil(t, output)
	assert.Len(t, output.Topics, 1)
	assert.Equal(t, "Tech News", output.Topics[0].Topic)
	assert.Equal(t, 1, output.ArticlesScanned)
	assert.False(t, output.GenerationInfo.Fallback)

	mockArticleClient.AssertExpectations(t)
	mockRetrieveUC.AssertExpectations(t)
	mockPromptBuilder.AssertExpectations(t)
	mockLLM.AssertExpectations(t)
}

func TestMorningLetterUsecase_Execute_NoArticles(t *testing.T) {
	mockArticleClient := new(MockArticleClient)
	mockRetrieveUC := new(mockRetrieveContextUsecase)
	mockPromptBuilder := new(MockMorningLetterPromptBuilder)
	mockLLM := new(mockLLMClient)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewMorningLetterUsecase(
		mockArticleClient,
		mockRetrieveUC,
		mockPromptBuilder,
		mockLLM,
		4096,
		6000,
		usecase.DefaultTemporalBoostConfig(),
		testLogger,
	)

	ctx := context.Background()
	input := usecase.MorningLetterInput{
		Query: "What are the important news?",
	}

	// Mock GetRecentArticles returning empty (limit=0 means no limit)
	mockArticleClient.On("GetRecentArticles", ctx, 24, 0).Return([]domain.ArticleMetadata{}, nil)

	output, err := uc.Execute(ctx, input)
	require.NoError(t, err)
	assert.Empty(t, output.Topics)
	assert.Equal(t, 0, output.ArticlesScanned)
	assert.True(t, output.GenerationInfo.Fallback)
	assert.Equal(t, "none", output.GenerationInfo.Model)
}

func TestMorningLetterUsecase_Execute_ArticleClientError(t *testing.T) {
	mockArticleClient := new(MockArticleClient)
	mockRetrieveUC := new(mockRetrieveContextUsecase)
	mockPromptBuilder := new(MockMorningLetterPromptBuilder)
	mockLLM := new(mockLLMClient)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewMorningLetterUsecase(
		mockArticleClient,
		mockRetrieveUC,
		mockPromptBuilder,
		mockLLM,
		4096,
		6000,
		usecase.DefaultTemporalBoostConfig(),
		testLogger,
	)

	ctx := context.Background()
	input := usecase.MorningLetterInput{
		Query: "What are the important news?",
	}

	// Mock GetRecentArticles returning error (limit=0 means no limit)
	mockArticleClient.On("GetRecentArticles", ctx, 24, 0).Return(nil, errors.New("connection failed"))

	_, err := uc.Execute(ctx, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch recent articles")
}

func TestMorningLetterUsecase_Execute_DefaultValues(t *testing.T) {
	mockArticleClient := new(MockArticleClient)
	mockRetrieveUC := new(mockRetrieveContextUsecase)
	mockPromptBuilder := new(MockMorningLetterPromptBuilder)
	mockLLM := new(mockLLMClient)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewMorningLetterUsecase(
		mockArticleClient,
		mockRetrieveUC,
		mockPromptBuilder,
		mockLLM,
		4096,
		6000,
		usecase.DefaultTemporalBoostConfig(),
		testLogger,
	)

	ctx := context.Background()
	input := usecase.MorningLetterInput{
		Query:       "test",
		WithinHours: 0,  // Should default to 24
		TopicLimit:  0,  // Should default to 5
		Locale:      "", // Should default to "ja"
	}

	// Mock GetRecentArticles - verify default withinHours=24 is used (limit=0 means no limit)
	mockArticleClient.On("GetRecentArticles", ctx, 24, 0).Return([]domain.ArticleMetadata{}, nil)

	output, err := uc.Execute(ctx, input)
	require.NoError(t, err)
	assert.True(t, output.GenerationInfo.Fallback)

	mockArticleClient.AssertExpectations(t)
}

func TestMorningLetterUsecase_Execute_MaxLimits(t *testing.T) {
	mockArticleClient := new(MockArticleClient)
	mockRetrieveUC := new(mockRetrieveContextUsecase)
	mockPromptBuilder := new(MockMorningLetterPromptBuilder)
	mockLLM := new(mockLLMClient)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewMorningLetterUsecase(
		mockArticleClient,
		mockRetrieveUC,
		mockPromptBuilder,
		mockLLM,
		4096,
		6000,
		usecase.DefaultTemporalBoostConfig(),
		testLogger,
	)

	ctx := context.Background()
	input := usecase.MorningLetterInput{
		Query:       "test",
		WithinHours: 500, // Should be capped to 168
		TopicLimit:  100, // Should be capped to 20
	}

	// Mock GetRecentArticles - verify capped withinHours=168 is used (limit=0 means no limit)
	mockArticleClient.On("GetRecentArticles", ctx, 168, 0).Return([]domain.ArticleMetadata{}, nil)

	output, err := uc.Execute(ctx, input)
	require.NoError(t, err)
	assert.True(t, output.GenerationInfo.Fallback)

	mockArticleClient.AssertExpectations(t)
}

func TestMorningLetterUsecase_Execute_MaxTokensPassedToLLM(t *testing.T) {
	mockArticleClient := new(MockArticleClient)
	mockRetrieveUC := new(mockRetrieveContextUsecase)
	mockPromptBuilder := new(MockMorningLetterPromptBuilder)
	mockLLM := new(mockLLMClient)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	customMaxTokens := 6144
	uc := usecase.NewMorningLetterUsecase(
		mockArticleClient,
		mockRetrieveUC,
		mockPromptBuilder,
		mockLLM,
		customMaxTokens,
		6000,
		usecase.DefaultTemporalBoostConfig(),
		testLogger,
	)

	ctx := context.Background()
	input := usecase.MorningLetterInput{
		Query:       "What are the important news?",
		WithinHours: 24,
		TopicLimit:  5,
		Locale:      "ja",
	}

	articleID := uuid.New()
	now := time.Now()

	mockArticleClient.On("GetRecentArticles", ctx, 24, 0).Return([]domain.ArticleMetadata{
		{
			ID:          articleID,
			Title:       "Test Article",
			URL:         "https://example.com/article",
			PublishedAt: now.Add(-2 * time.Hour),
			FeedID:      uuid.New(),
			Tags:        []string{"tech"},
		},
	}, nil)

	mockRetrieveUC.On("Execute", ctx, mock.MatchedBy(func(input usecase.RetrieveContextInput) bool {
		return input.Query == "What are the important news?"
	})).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkText:   "Test article content.",
				URL:         "https://example.com/article",
				Title:       "Test Article",
				PublishedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
				Score:       0.95,
			},
		},
	}, nil)

	mockPromptBuilder.On("Build", mock.AnythingOfType("usecase.MorningLetterPromptInput")).Return([]domain.Message{
		{Role: "system", Content: "You are a news analyst..."},
		{Role: "user", Content: "Analyze these articles..."},
	}, nil)

	// Verify customMaxTokens is passed through to LLM
	mockLLM.On("Chat", ctx, mock.AnythingOfType("[]domain.Message"), customMaxTokens).Return(&domain.LLMResponse{
		Text: `{"topics": [{"topic": "Tech", "headline": "News", "summary": "Summary", "importance": 0.9, "article_refs": [], "keywords": ["tech"]}], "meta": {"topics_found": 1, "coverage_assessment": "ok"}}`,
		Done: true,
	}, nil)

	output, err := uc.Execute(ctx, input)
	require.NoError(t, err)
	assert.NotNil(t, output)
	assert.Len(t, output.Topics, 1)

	mockLLM.AssertExpectations(t)
}

func TestMorningLetterUsecase_Execute_ContextTokenLimiting(t *testing.T) {
	mockArticleClient := new(MockArticleClient)
	mockRetrieveUC := new(mockRetrieveContextUsecase)
	mockPromptBuilder := new(MockMorningLetterPromptBuilder)
	mockLLM := new(mockLLMClient)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	uc := usecase.NewMorningLetterUsecase(
		mockArticleClient,
		mockRetrieveUC,
		mockPromptBuilder,
		mockLLM,
		4096,
		6000,
		usecase.DefaultTemporalBoostConfig(),
		testLogger,
	)

	ctx := context.Background()
	input := usecase.MorningLetterInput{
		Query:       "What are the important news?",
		WithinHours: 24,
		TopicLimit:  5,
		Locale:      "ja",
	}

	articleID := uuid.New()
	now := time.Now()

	mockArticleClient.On("GetRecentArticles", ctx, 24, 0).Return([]domain.ArticleMetadata{
		{ID: articleID, Title: "Test", URL: "https://example.com", PublishedAt: now.Add(-1 * time.Hour), FeedID: uuid.New()},
	}, nil)

	// Create contexts with very large chunks that exceed token limit
	// maxMorningLetterPromptTokens = 6000, overhead = 600
	// So ~5400 tokens available = ~16200 chars
	largeChunk := make([]byte, 15000) // ~5000 tokens each
	for i := range largeChunk {
		largeChunk[i] = 'a'
	}
	largeChunkStr := string(largeChunk)

	mockRetrieveUC.On("Execute", ctx, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkText: largeChunkStr, URL: "https://example.com/1", Title: "Article 1", PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339), Score: 0.95},
			{ChunkText: largeChunkStr, URL: "https://example.com/2", Title: "Article 2", PublishedAt: now.Add(-2 * time.Hour).Format(time.RFC3339), Score: 0.90},
			{ChunkText: largeChunkStr, URL: "https://example.com/3", Title: "Article 3", PublishedAt: now.Add(-3 * time.Hour).Format(time.RFC3339), Score: 0.85},
		},
	}, nil)

	// PromptBuilder should receive limited contexts (not all 3)
	mockPromptBuilder.On("Build", mock.MatchedBy(func(input usecase.MorningLetterPromptInput) bool {
		// With 15000 chars per chunk (~5000 tokens) and overhead=600, max=6000 tokens:
		// First chunk: 600+5000=5600 < 6000 -> include
		// Second chunk: 5600+5000=10600 > 6000 -> stop
		return len(input.Contexts) <= 2
	})).Return([]domain.Message{
		{Role: "system", Content: "analyst"},
		{Role: "user", Content: "analyze"},
	}, nil)

	mockLLM.On("Chat", ctx, mock.AnythingOfType("[]domain.Message"), 4096).Return(&domain.LLMResponse{
		Text: `{"topics": [{"topic": "Tech", "headline": "News", "summary": "Summary", "importance": 0.9, "article_refs": [], "keywords": ["tech"]}], "meta": {"topics_found": 1, "coverage_assessment": "ok"}}`,
		Done: true,
	}, nil)

	output, err := uc.Execute(ctx, input)
	require.NoError(t, err)
	assert.NotNil(t, output)

	mockPromptBuilder.AssertExpectations(t)
}

func TestMorningLetterUsecase_MaxTokensDefaultsWhenZero(t *testing.T) {
	mockArticleClient := new(MockArticleClient)
	mockRetrieveUC := new(mockRetrieveContextUsecase)
	mockPromptBuilder := new(MockMorningLetterPromptBuilder)
	mockLLM := new(mockLLMClient)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Pass 0 -> should default to 4096
	uc := usecase.NewMorningLetterUsecase(
		mockArticleClient,
		mockRetrieveUC,
		mockPromptBuilder,
		mockLLM,
		0,
		0,
		usecase.DefaultTemporalBoostConfig(),
		testLogger,
	)

	ctx := context.Background()
	input := usecase.MorningLetterInput{
		Query:       "test",
		WithinHours: 24,
		Locale:      "ja",
	}

	articleID := uuid.New()
	now := time.Now()

	mockArticleClient.On("GetRecentArticles", ctx, 24, 0).Return([]domain.ArticleMetadata{
		{ID: articleID, Title: "Test", URL: "https://example.com", PublishedAt: now.Add(-1 * time.Hour), FeedID: uuid.New()},
	}, nil)

	mockRetrieveUC.On("Execute", ctx, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{ChunkText: "content", URL: "https://example.com", Title: "Test", PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339), Score: 0.9},
		},
	}, nil)

	mockPromptBuilder.On("Build", mock.Anything).Return([]domain.Message{
		{Role: "system", Content: "analyst"},
		{Role: "user", Content: "analyze"},
	}, nil)

	// Should use default 4096
	mockLLM.On("Chat", ctx, mock.AnythingOfType("[]domain.Message"), 4096).Return(&domain.LLMResponse{
		Text: `{"topics": [], "meta": {"topics_found": 0, "coverage_assessment": "none"}}`,
		Done: true,
	}, nil)

	_, err := uc.Execute(ctx, input)
	require.NoError(t, err)
	mockLLM.AssertExpectations(t)
}
