package domain_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestChunker_Chunk(t *testing.T) {
	chunker := domain.NewChunker()

	t.Run("Splits by paragraphs and merges short ones", func(t *testing.T) {
		// Short paragraphs will be merged due to MinChunkLength
		body := "Paragraph 1.\n\nParagraph 2.\n\nParagraph 3."
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)
		// All three short paragraphs will be merged into one chunk
		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "Paragraph 1.")
		assert.Contains(t, chunks[0].Content, "Paragraph 2.")
		assert.Contains(t, chunks[0].Content, "Paragraph 3.")
		assert.Equal(t, 0, chunks[0].Ordinal)
	})

	t.Run("Ignores empty paragraphs and merges short ones", func(t *testing.T) {
		body := "Para 1\n\n\n\nPara 2"
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)
		// Short paragraphs will be merged
		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "Para 1")
		assert.Contains(t, chunks[0].Content, "Para 2")
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

	t.Run("Trims whitespace and merges short chunks", func(t *testing.T) {
		body := "  Para 1  \n\n  Para 2  "
		chunks, _ := chunker.Chunk(body)
		// Short paragraphs will be merged
		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "Para 1")
		assert.Contains(t, chunks[0].Content, "Para 2")
	})

	t.Run("Merges short chunks", func(t *testing.T) {
		// Create short paragraphs that should be merged with the following long paragraph
		body := "Short.\n\nAlso short.\n\nThis is a longer paragraph that exceeds the minimum chunk length and should stand alone."
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)

		// v5: Short paragraphs at start get prepended to first long paragraph
		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "Short.")
		assert.Contains(t, chunks[0].Content, "Also short.")
		assert.Contains(t, chunks[0].Content, "This is a longer paragraph")
	})

	t.Run("Splits long chunks at sentence boundaries", func(t *testing.T) {
		// Create a very long paragraph
		longText := ""
		for i := 0; i < 20; i++ {
			longText += "This is sentence number " + string(rune('0'+i)) + " in a very long paragraph. "
		}

		chunks, err := chunker.Chunk(longText)
		assert.NoError(t, err)

		// Should split into multiple chunks
		assert.Greater(t, len(chunks), 1)

		// Each chunk should be under MaxChunkLength
		for _, chunk := range chunks {
			assert.LessOrEqual(t, len(chunk.Content), domain.MaxChunkLength)
		}
	})

	t.Run("Handles mixed short and long paragraphs", func(t *testing.T) {
		shortPara := "Tag 1"
		longPara := "This is a very long paragraph that contains enough text to exceed the minimum chunk length requirement. "
		longPara += "It has multiple sentences to ensure proper handling. "
		longPara += "This should remain as a separate chunk because it's long enough."

		body := shortPara + "\n\n" + "Tag 2" + "\n\n" + longPara

		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)

		// Should have merged short tags and kept long paragraph separate
		assert.GreaterOrEqual(t, len(chunks), 1)

		// At least one chunk should be >= MinChunkLength
		hasLongChunk := false
		for _, chunk := range chunks {
			if len(chunk.Content) >= domain.MinChunkLength {
				hasLongChunk = true
				break
			}
		}
		assert.True(t, hasLongChunk)
	})

	t.Run("Returns v7 version", func(t *testing.T) {
		assert.Equal(t, domain.ChunkerVersionV7, chunker.Version())
	})

	t.Run("Handles Japanese sentence boundaries", func(t *testing.T) {
		// Japanese text with 。 as sentence terminator
		body := "これは最初の文です。これは二番目の文です。これは三番目の文です。"
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)
		assert.NotEmpty(t, chunks)
	})

	t.Run("Keeps long article content separate from navigation fragments", func(t *testing.T) {
		// Simulate NHK-style navigation fragments
		body := "注目ワード\n\nあわせて読みたい\n\n" +
			"This is the actual article content with sufficient length. " +
			"It contains multiple sentences and provides meaningful context. " +
			"This should be preserved in the chunks."

		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)

		// v5: Short navigation fragments get prepended to article content
		assert.Len(t, chunks, 1)
		// Chunk contains both navigation and article content
		assert.Contains(t, chunks[0].Content, "注目ワード")
		assert.Contains(t, chunks[0].Content, "あわせて読みたい")
		assert.Contains(t, chunks[0].Content, "This is the actual article content")
	})

	t.Run("Merges leading short title with following navigation items", func(t *testing.T) {
		// Simulates title (27 chars) + navigation menu (multiple short items) + first long paragraph
		// This reproduces the real-world case where title is embedded in body
		// Note: MinChunkLength is now 80, so we need slightly longer content to test boundary
		body := "Short Title Here 1234567890\n\nMenu1\n\nMenu2\n\nMenu3\n\nThis is a long enough paragraph that exceeds the minimum chunk length requirement of 80 characters. It needs to be a bit longer now."
		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)

		// All chunks should be >= MinChunkLength
		for i, chunk := range chunks {
			if len(chunk.Content) < domain.MinChunkLength {
				t.Errorf("Chunk %d is too short (%d chars): %q", i, len(chunk.Content), chunk.Content)
			}
		}

		// v5/v6: Short items get prepended to first long paragraph, creating 1 chunk
		assert.Len(t, chunks, 1)
		assert.Contains(t, chunks[0].Content, "Short Title")
		assert.Contains(t, chunks[0].Content, "Menu1")
		assert.Contains(t, chunks[0].Content, "This is a long enough paragraph")
	})

	t.Run("Merges consecutive short chunks after long paragraph", func(t *testing.T) {
		// Pattern: Long -> Short -> Short -> Long
		// C and D should be merged with E
		// Long A (must be >= 80)
		longA := strings.Repeat("A", 85)
		shortC := "Short C"
		shortD := "Short D"
		// Long E (must be >= 80)
		longE := strings.Repeat("E", 85)

		body := longA + "\n\n" + shortC + "\n\n" + shortD + "\n\n" + longE

		chunks, err := chunker.Chunk(body)
		assert.NoError(t, err)

		// All chunks should be >= MinChunkLength (80)
		for i, chunk := range chunks {
			runes := utf8.RuneCountInString(chunk.Content)
			assert.GreaterOrEqual(t, runes, domain.MinChunkLength,
				"Chunk %d too short: %d chars", i, runes)
		}

		// Expect 2 chunks:
		// 1. Long A + Short C + Short D (short chunks merged with previous long chunk A which is preferred over next E)
		// 2. Long E
		assert.Len(t, chunks, 2)
		assert.Contains(t, chunks[0].Content, "Short C")
		assert.Contains(t, chunks[0].Content, "Short D")
		assert.Contains(t, chunks[1].Content, strings.Repeat("E", 10))
	})

	t.Run("MinChunkLength is 80", func(t *testing.T) {
		assert.Equal(t, 80, domain.MinChunkLength)
	})
}
