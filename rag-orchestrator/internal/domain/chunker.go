package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// ChunkerVersion defines the version of the chunking algorithm.
// This allows for future upgrades while tracking which version was used.
type ChunkerVersion string

const (
	// ChunkerVersionV1 is the initial paragraph-based chunker.
	ChunkerVersionV1 ChunkerVersion = "v1"
)

// Chunk represents a single piece of a document.
type Chunk struct {
	Ordinal int    // Sequence number (0-indexed)
	Content string // The actual text content
	Hash    string // Stable hash of the content (SHA-256)
}

// Chunker defines the interface for splitting text into chunks.
type Chunker interface {
	Chunk(body string) ([]Chunk, error)
	Version() ChunkerVersion
}

type paragraphChunker struct{}

// NewChunker creates a new instance of the default Chunker (Version 1).
func NewChunker() Chunker {
	return &paragraphChunker{}
}

func (c *paragraphChunker) Version() ChunkerVersion {
	return ChunkerVersionV1
}

// Chunk splits the body into chunks based on double newlines (paragraphs).
// It trims whitespace from each chunk and ignores empty chunks.
func (c *paragraphChunker) Chunk(body string) ([]Chunk, error) {
	// Normalize newlines to \n
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	// Split by double newline to get paragraphs
	parts := strings.Split(normalized, "\n\n")

	var chunks []Chunk
	ordinal := 0

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		// Compute hash
		hashBytes := sha256.Sum256([]byte(trimmed))
		hash := hex.EncodeToString(hashBytes[:])

		chunks = append(chunks, Chunk{
			Ordinal: ordinal,
			Content: trimmed,
			Hash:    hash,
		})
		ordinal++
	}

	return chunks, nil
}
