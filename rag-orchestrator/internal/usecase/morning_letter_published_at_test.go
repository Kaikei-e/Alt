package usecase_test

import (
	"context"
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

// TestMorningLetterUsecase_Execute_BoostsAndCitesByArticlePublishedAt pins the
// post-reindex behaviour. ContextItem.PublishedAt is filled from
// rag_chunks.created_at (see retrieval/allocate.go), which is the index time,
// not the publication date — after a re-index every chunk looks equally fresh
// and the recency boost degenerates into "unchanged retrieval order". The
// authoritative published_at is the one alt-backend already returned in
// ArticleMetadata, and it must drive both the boost and the citation dates.
func TestMorningLetterUsecase_Execute_BoostsAndCitesByArticlePublishedAt(t *testing.T) {
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
		WithinHours: 168,
		TopicLimit:  5,
		Locale:      "ja",
	}

	now := time.Now()
	// The whole corpus was re-indexed 10 minutes ago, so every chunk carries
	// the same created_at regardless of when its article was published.
	indexedAt := now.Add(-10 * time.Minute).Truncate(time.Second)
	freshPublished := now.Add(-2 * time.Hour).Truncate(time.Second)   // 1.3x boost
	stalePublished := now.Add(-100 * time.Hour).Truncate(time.Second) // no boost

	freshID := uuid.New()
	staleID := uuid.New()

	mockArticleClient.On("GetRecentArticles", ctx, 168, 0).Return([]domain.ArticleMetadata{
		{ID: freshID, Title: "Fresh Article", URL: "https://example.com/fresh", PublishedAt: freshPublished, FeedID: uuid.New()},
		{ID: staleID, Title: "Old Article", URL: "https://example.com/old", PublishedAt: stalePublished, FeedID: uuid.New()},
	}, nil)

	// Raw retrieval ranks the old article first (0.90 > 0.80). Boosting on the
	// real publication dates flips it: 0.80*1.3 = 1.04 > 0.90*1.0.
	mockRetrieveUC.On("Execute", ctx, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkText:   "Old article content.",
				URL:         "https://example.com/old",
				Title:       "Old Article",
				PublishedAt: indexedAt.Format(time.RFC3339),
				Score:       0.90,
				ArticleID:   staleID.String(),
			},
			{
				ChunkText:   "Fresh article content.",
				URL:         "https://example.com/fresh",
				Title:       "Fresh Article",
				PublishedAt: indexedAt.Format(time.RFC3339),
				Score:       0.80,
				ArticleID:   freshID.String(),
			},
		},
	}, nil)

	var promptContexts []usecase.ContextItem
	mockPromptBuilder.On("Build", mock.AnythingOfType("usecase.MorningLetterPromptInput")).
		Run(func(args mock.Arguments) {
			promptContexts = args.Get(0).(usecase.MorningLetterPromptInput).Contexts
		}).
		Return([]domain.Message{
			{Role: "system", Content: "You are a news analyst..."},
			{Role: "user", Content: "Analyze these articles..."},
		}, nil)

	mockLLM.On("Chat", ctx, mock.AnythingOfType("[]domain.Message"), 4096).Return(&domain.LLMResponse{
		Text: `{
			"topics": [
				{
					"topic": "Tech News",
					"headline": "Major tech announcement",
					"summary": "A significant tech development was announced today.",
					"importance": 0.9,
					"article_refs": [1],
					"keywords": ["tech"]
				}
			],
			"meta": {"topics_found": 1, "coverage_assessment": "comprehensive"}
		}`,
		Done: true,
	}, nil)

	output, err := uc.Execute(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)

	require.Len(t, promptContexts, 2)
	assert.Equal(t, "Fresh Article", promptContexts[0].Title,
		"the 2h-old article must outrank the 100h-old one; the boost must not read the shared index time")
	assert.Equal(t, freshPublished.Format(time.RFC3339), promptContexts[0].PublishedAt,
		"the LLM must be shown the publication date, not the index date")
	assert.Equal(t, stalePublished.Format(time.RFC3339), promptContexts[1].PublishedAt)

	require.Len(t, output.Topics, 1)
	refs := output.Topics[0].ArticleRefs
	require.Len(t, refs, 1)
	assert.Equal(t, freshID, refs[0].ID)
	assert.WithinDuration(t, freshPublished, refs[0].PublishedAt, time.Second,
		"citation date must be the publication date, not the index date")
}

// TestMorningLetterUsecase_Execute_DoesNotBoostUnknownPublishedAt covers the
// context whose article alt-backend did not report: with no publication date
// there is nothing to boost by, and the index time must not be passed off as
// one.
func TestMorningLetterUsecase_Execute_DoesNotBoostUnknownPublishedAt(t *testing.T) {
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

	now := time.Now()
	indexedAt := now.Add(-5 * time.Minute).Truncate(time.Second)
	knownPublished := now.Add(-1 * time.Hour).Truncate(time.Second) // 1.3x boost

	knownID := uuid.New()

	mockArticleClient.On("GetRecentArticles", ctx, 24, 0).Return([]domain.ArticleMetadata{
		{ID: knownID, Title: "Known Article", URL: "https://example.com/known", PublishedAt: knownPublished, FeedID: uuid.New()},
	}, nil)

	// The orphan chunk outranks the known one on raw score (0.90 > 0.80) but
	// must stay unboosted, so the known article's 0.80*1.3 = 1.04 wins.
	mockRetrieveUC.On("Execute", ctx, mock.Anything).Return(&usecase.RetrieveContextOutput{
		Contexts: []usecase.ContextItem{
			{
				ChunkText:   "Orphan chunk content.",
				URL:         "https://example.com/orphan",
				Title:       "Orphan Article",
				PublishedAt: indexedAt.Format(time.RFC3339),
				Score:       0.90,
				ArticleID:   uuid.New().String(),
			},
			{
				ChunkText:   "Known article content.",
				URL:         "https://example.com/known",
				Title:       "Known Article",
				PublishedAt: indexedAt.Format(time.RFC3339),
				Score:       0.80,
				ArticleID:   knownID.String(),
			},
		},
	}, nil)

	var promptContexts []usecase.ContextItem
	mockPromptBuilder.On("Build", mock.AnythingOfType("usecase.MorningLetterPromptInput")).
		Run(func(args mock.Arguments) {
			promptContexts = args.Get(0).(usecase.MorningLetterPromptInput).Contexts
		}).
		Return([]domain.Message{
			{Role: "system", Content: "You are a news analyst..."},
			{Role: "user", Content: "Analyze these articles..."},
		}, nil)

	mockLLM.On("Chat", ctx, mock.AnythingOfType("[]domain.Message"), 4096).Return(&domain.LLMResponse{
		Text: `{"topics": [{"topic": "Tech", "headline": "News", "summary": "Summary", "importance": 0.9, "article_refs": [2], "keywords": ["tech"]}], "meta": {"topics_found": 1, "coverage_assessment": "partial"}}`,
		Done: true,
	}, nil)

	output, err := uc.Execute(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)

	require.Len(t, promptContexts, 2)
	assert.Equal(t, "Known Article", promptContexts[0].Title,
		"an article with no known publication date must not be boosted ahead of a genuinely recent one")
	assert.Empty(t, promptContexts[1].PublishedAt,
		"an unknown publication date must be omitted rather than reported as the index date")

	require.Len(t, output.Topics, 1)
	refs := output.Topics[0].ArticleRefs
	require.Len(t, refs, 1)
	assert.True(t, refs[0].PublishedAt.IsZero(),
		"a citation for an article with no known publication date must not carry the index date")
}
