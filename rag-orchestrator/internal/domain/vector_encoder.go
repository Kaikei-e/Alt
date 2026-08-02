package domain

import (
	"context"
	"fmt"
)

// EmbeddingDimension is the width every stored chunk embedding must have.
// It is pinned to the rag_chunks.embedding column type (vector(1024), served
// by bge-m3): the schema, not the environment, is the authority here, so a
// vector of any other width means the embedder is running a different model
// than the index was built from.
const EmbeddingDimension = 1024

// EmbedderVersion identifies the vector space a stored embedding belongs to:
// the model that produced it, plus the width it was stored at. It is the
// embedding-side counterpart of ChunkerVersion — when it changes, every
// embedding derived from the old value is meaningless and the article must be
// re-indexed. Deriving it from the configured model (rather than a
// hand-bumped constant) is what makes a future EMBEDDING_MODEL swap
// re-trigger indexing on its own.
func EmbedderVersion(model string) string {
	return fmt.Sprintf("%s/%d", model, EmbeddingDimension)
}

// VectorEncoder defines the interface for generating embeddings.
type VectorEncoder interface {
	Encode(ctx context.Context, texts []string) ([][]float32, error)
	Version() string
}
