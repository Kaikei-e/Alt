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
	mockLLM.On("Chat", ctx, mock.AnythingOfType("[]domain.Message"), 1500).Return(&domain.LLMResponse{
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
