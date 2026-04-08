package eino

import (
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

func TestToEinoMessages_RoleMapping(t *testing.T) {
	msgs := []domain.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "user query"},
		{Role: "assistant", Content: "assistant response"},
		{Role: "tool", Content: "tool result"},
	}

	result := toEinoMessages(msgs)
	assert.Len(t, result, 4)
	assert.Equal(t, schema.System, result[0].Role)
	assert.Equal(t, schema.User, result[1].Role)
	assert.Equal(t, schema.Assistant, result[2].Role)
	assert.Equal(t, schema.Tool, result[3].Role)
}

func TestToEinoMessages_ContentPreserved(t *testing.T) {
	msgs := []domain.Message{
		{Role: "user", Content: "テスト質問"},
	}

	result := toEinoMessages(msgs)
	assert.Equal(t, "テスト質問", result[0].Content)
}

func TestToEinoMessages_Empty(t *testing.T) {
	result := toEinoMessages(nil)
	assert.Empty(t, result)
}

// Compile-time interface check: ChatModelAdapter must implement domain.LLMClient.
var _ domain.LLMClient = (*ChatModelAdapter)(nil)
var _ domain.ToolCallingLLMClient = (*ChatModelAdapter)(nil)
