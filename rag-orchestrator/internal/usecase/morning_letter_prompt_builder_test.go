package usecase_test

import (
	"rag-orchestrator/internal/usecase"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMorningLetterPromptBuilder_Build_Success(t *testing.T) {
	builder := usecase.NewMorningLetterPromptBuilder()

	now := time.Now()
	since := now.Add(-24 * time.Hour)

	input := usecase.MorningLetterPromptInput{
		Query: "What are the important news from yesterday?",
		Contexts: []usecase.ContextItem{
			{
				ChunkText:   "Breaking news: Tech company announces new product.",
				URL:         "https://example.com/news1",
				Title:       "Tech Announcement",
				PublishedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
				Score:       0.95,
			},
			{
				ChunkText:   "Market analysis shows positive trends.",
				URL:         "https://example.com/news2",
				Title:       "Market Analysis",
				PublishedAt: now.Add(-5 * time.Hour).Format(time.RFC3339),
				Score:       0.88,
			},
		},
		Since:      since,
		Until:      now,
		TopicLimit: 5,
		Locale:     "ja",
	}

	messages, err := builder.Build(input)
	require.NoError(t, err)
	require.Len(t, messages, 2)

	// Check system message (Japanese for Gemma 3 token efficiency)
	assert.Equal(t, "system", messages[0].Role)
	assert.Contains(t, messages[0].Content, "ニュースアナリスト")
	assert.Contains(t, messages[0].Content, "24時間")
	assert.Contains(t, messages[0].Content, "最大5個") // topic limit

	// Check user message
	assert.Equal(t, "user", messages[1].Role)
	assert.Contains(t, messages[1].Content, "Tech Announcement")
	assert.Contains(t, messages[1].Content, "Market Analysis")
	assert.Contains(t, messages[1].Content, "What are the important news from yesterday?")
	assert.Contains(t, messages[1].Content, "ja")
}

func TestMorningLetterPromptBuilder_Build_EmptyContexts(t *testing.T) {
	builder := usecase.NewMorningLetterPromptBuilder()

	input := usecase.MorningLetterPromptInput{
		Query:    "test query",
		Contexts: []usecase.ContextItem{},
		Since:    time.Now().Add(-24 * time.Hour),
		Until:    time.Now(),
	}

	_, err := builder.Build(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no contexts provided")
}

func TestMorningLetterPromptBuilder_Build_DefaultTopicLimit(t *testing.T) {
	builder := usecase.NewMorningLetterPromptBuilder()

	now := time.Now()
	since := now.Add(-24 * time.Hour)

	input := usecase.MorningLetterPromptInput{
		Query: "test query",
		Contexts: []usecase.ContextItem{
			{
				ChunkText:   "Some news content",
				URL:         "https://example.com/news",
				Title:       "News Title",
				PublishedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
				Score:       0.90,
			},
		},
		Since:      since,
		Until:      now,
		TopicLimit: 0, // Should default to 5
	}

	messages, err := builder.Build(input)
	require.NoError(t, err)
	require.Len(t, messages, 2)

	// Should default to 10 topics
	assert.Contains(t, messages[0].Content, "最大10個")
}
