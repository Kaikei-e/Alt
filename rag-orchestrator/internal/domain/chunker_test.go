package domain_test

import (
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestChunker_Chunk(t *testing.T) {
	chunker := domain.NewChunker()

	t.Run("Splits by paragraphs", func(t *testing.T) {
		body := "Paragraph 1.\n\nParagraph 2.\n\nParagraph 3."
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)
		assert.Len(t, chunks, 3)
		assert.Equal(t, "Paragraph 1.", chunks[0].Content)
		assert.Equal(t, 0, chunks[0].Ordinal)
		assert.Equal(t, "Paragraph 2.", chunks[1].Content)
		assert.Equal(t, 1, chunks[1].Ordinal)
		assert.Equal(t, "Paragraph 3.", chunks[2].Content)
		assert.Equal(t, 2, chunks[2].Ordinal)
	})

	t.Run("Ignores empty paragraphs", func(t *testing.T) {
		body := "Para 1\n\n\n\nPara 2"
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)
		assert.Len(t, chunks, 2)
		assert.Equal(t, "Para 1", chunks[0].Content)
		assert.Equal(t, "Para 2", chunks[1].Content)
	})

	t.Run("Computes stable hash", func(t *testing.T) {
		body := "Content"
		chunks1, _ := chunker.Chunk(body)
		chunks2, _ := chunker.Chunk(body)

		assert.NotEmpty(t, chunks1[0].Hash)
		assert.Equal(t, chunks1[0].Hash, chunks2[0].Hash)
	})

	t.Run("Handles single line", func(t *testing.T) {
		body := "Single line."
		chunks, _ := chunker.Chunk(body)
		assert.Len(t, chunks, 1)
		assert.Equal(t, "Single line.", chunks[0].Content)
	})

	t.Run("Trims whitespace", func(t *testing.T) {
		body := "  Para 1  \n\n  Para 2  "
		chunks, _ := chunker.Chunk(body)
		assert.Equal(t, "Para 1", chunks[0].Content)
		assert.Equal(t, "Para 2", chunks[1].Content)
	})
}
