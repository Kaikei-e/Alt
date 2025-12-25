package domain

import (
	"context"
)

// VectorEncoder defines the interface for generating embeddings.
type VectorEncoder interface {
	Encode(ctx context.Context, texts []string) ([][]float32, error)
	Version() string
}
