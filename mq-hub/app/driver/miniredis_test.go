package driver

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
)

// Miniredis wraps miniredis for testing.
type Miniredis struct {
	*miniredis.Miniredis
}

// NewMiniredis creates a new miniredis instance for testing.
func NewMiniredis(t *testing.T) *Miniredis {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	return &Miniredis{Miniredis: mr}
}
