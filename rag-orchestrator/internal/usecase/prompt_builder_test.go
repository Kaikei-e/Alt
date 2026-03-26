package usecase_test

import (
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptBuilder_MultiTurnReturnsChatMessages(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "もっと詳しく教えて",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content about the topic"},
		},
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "この記事の要点は？"},
			{Role: "assistant", Content: "この記事は新しいプロトコルについて説明しています。"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	// Multi-turn should return multiple messages (past turns + current)
	assert.Greater(t, len(msgs), 1,
		"multi-turn should produce multiple chat messages, not a single message")

	// Past turns should be actual chat messages
	assert.Equal(t, "user", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "この記事の要点は？")
	assert.Equal(t, "assistant", msgs[1].Role)
	assert.Contains(t, msgs[1].Content, "新しいプロトコルについて")

	// Last message should be user with follow-up context + query
	lastMsg := msgs[len(msgs)-1]
	assert.Equal(t, "user", lastMsg.Role)
	assert.Contains(t, lastMsg.Content, "もっと詳しく教えて")
	assert.Contains(t, lastMsg.Content, "Content about the topic",
		"current turn should include context chunks")
}

func TestPromptBuilder_MultiTurnFollowUpInstructions(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "深掘りして",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
		ConversationHistory: []domain.Message{
			{Role: "user", Content: "要点は？"},
			{Role: "assistant", Content: "回答テキスト"},
		},
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	lastMsg := msgs[len(msgs)-1]
	assert.Contains(t, lastMsg.Content, "繰り返さない",
		"follow-up should instruct not to repeat")
	assert.Contains(t, lastMsg.Content, "直接回答",
		"follow-up should request direct answer without overview")
	assert.Contains(t, lastMsg.Content, "日本語",
		"follow-up should enforce Japanese response")
	assert.NotContains(t, lastMsg.Content, "## 概要\\n...\\n## 詳細",
		"follow-up should NOT include overview/detail/summary format")
}

func TestPromptBuilder_SingleTurnUnchanged(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "この記事の要点は？",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
		ConversationHistory: nil,
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	// Single-turn should still return 1 message
	assert.Len(t, msgs, 1, "single-turn should return exactly 1 message")
	assert.Equal(t, "user", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "あなたの役割",
		"single-turn should include full instructions")
}

func TestPromptBuilder_NoHistoryOmitsConversationSection(t *testing.T) {
	builder := usecase.NewXMLPromptBuilder()
	input := usecase.PromptInput{
		Query:         "この記事の要点は？",
		Locale:        "ja",
		PromptVersion: "v1",
		Contexts: []usecase.PromptContext{
			{ChunkID: "1", Title: "Test", ChunkText: "Content"},
		},
		ConversationHistory: nil,
	}

	msgs, err := builder.Build(input)
	require.NoError(t, err)

	assert.NotContains(t, msgs[0].Content, "会話履歴",
		"prompt without history should not contain conversation history section")
	assert.NotContains(t, msgs[0].Content, "フォローアップ",
		"prompt without history should not contain follow-up instructions")
}
