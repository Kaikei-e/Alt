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
	// ChunkerVersionV2 is the improved chunker with min/max length constraints.
	ChunkerVersionV2 ChunkerVersion = "v2"
	// ChunkerVersionV3 is v2 with trailing short chunk handling.
	ChunkerVersionV3 ChunkerVersion = "v3"
	// ChunkerVersionV4 fixes mid-stream short chunk handling.
	ChunkerVersionV4 ChunkerVersion = "v4"
	// ChunkerVersionV5 fixes leading short chunks by prepending to first long paragraph.
	ChunkerVersionV5 ChunkerVersion = "v5"
	// ChunkerVersionV6 improves consecutive short chunk merging and raises MinChunkLength to 80.
	ChunkerVersionV6 ChunkerVersion = "v6"
)

const (
	// MinChunkLength is the minimum allowed chunk length in characters.
	// Chunks shorter than this will be merged with adjacent chunks.
	MinChunkLength = 80
	// MaxChunkLength is the maximum allowed chunk length in characters.
	// Chunks longer than this will be split at sentence boundaries.
	MaxChunkLength = 1000
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
	return ChunkerVersionV6
}

// Chunk splits the body into chunks based on double newlines (paragraphs).
// It trims whitespace from each chunk and ignores empty chunks.
// Short chunks are merged with adjacent chunks, and long chunks are split.
func (c *paragraphChunker) Chunk(body string) ([]Chunk, error) {
	// Normalize newlines to \n
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	// Split by double newline to get paragraphs
	parts := strings.Split(normalized, "\n\n")

	// Extract non-empty paragraphs
	var paragraphs []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			paragraphs = append(paragraphs, trimmed)
		}
	}

	// Merge short chunks (First pass)
	merged := mergeShortChunks(paragraphs)

	// Merge consecutive short chunks (Second pass for v6)
	merged = mergeConsecutiveShortChunks(merged)

	// Split long chunks
	split := splitLongChunks(merged)

	// Create final chunks with hashes
	var chunks []Chunk
	for i, content := range split {
		hashBytes := sha256.Sum256([]byte(content))
		hash := hex.EncodeToString(hashBytes[:])

		chunks = append(chunks, Chunk{
			Ordinal: i,
			Content: content,
			Hash:    hash,
		})
	}

	return chunks, nil
}
