package domain

import (
	"strings"
	"unicode/utf8"
)

// splitLongChunks splits paragraphs longer than MaxChunkLength at sentence boundaries.
func splitLongChunks(paragraphs []string) []string {
	var result []string

	for _, para := range paragraphs {
		if utf8.RuneCountInString(para) <= MaxChunkLength {
			result = append(result, para)
			continue
		}

		// Split long paragraph at sentence boundaries
		sentences := splitIntoSentences(para)
		var chunk string

		for _, sentence := range sentences {
			// If adding this sentence would exceed max length, save current chunk
			// We check if (chunk + space + sentence) > MaxChunkLength
			// len(chunk) + 1 + len(sentence) for Space, but we need Rune count

			// Calculate projected length
			chunkLen := utf8.RuneCountInString(chunk)
			sentenceLen := utf8.RuneCountInString(sentence)
			spaceLen := 0
			if chunkLen > 0 {
				spaceLen = 1 // Space
			}

			if chunkLen > 0 && chunkLen+spaceLen+sentenceLen > MaxChunkLength {
				result = append(result, chunk)
				chunk = sentence
			} else {
				if chunk != "" {
					chunk += " "
				}
				chunk += sentence
			}
		}

		// Add remaining chunk
		if chunk != "" {
			result = append(result, chunk)
		}
	}

	return result
}

// splitIntoSentences splits text into sentences at common sentence boundaries.
func splitIntoSentences(text string) []string {
	// Simple sentence splitting at . ! ? followed by space or newline
	// Also handles Japanese period 。
	var sentences []string
	var current string

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		current += string(runes[i])

		// Check for sentence ending
		if runes[i] == '.' || runes[i] == '!' || runes[i] == '?' || runes[i] == '。' {
			// Look ahead to see if followed by space/newline or end of text
			if i+1 >= len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n' {
				sentences = append(sentences, strings.TrimSpace(current))
				current = ""
			}
		}
	}

	// Add remaining text as final sentence
	if trimmed := strings.TrimSpace(current); trimmed != "" {
		sentences = append(sentences, trimmed)
	}

	return sentences
}
