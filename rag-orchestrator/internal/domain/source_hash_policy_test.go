package domain_test

import (
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestSourceHashPolicy_Compute(t *testing.T) {
	policy := domain.NewSourceHashPolicy()

	t.Run("Same input produces same hash", func(t *testing.T) {
		h1 := policy.Compute("Title", "Body content")
		h2 := policy.Compute("Title", "Body content")
		assert.Equal(t, h1, h2)
	})

	t.Run("Whitespace differences are normalized", func(t *testing.T) {
		h1 := policy.Compute("Title", "Body content")
		h2 := policy.Compute("  Title  ", "\nBody content\n")
		assert.Equal(t, h1, h2)
	})

	t.Run("Different content produces different hash", func(t *testing.T) {
		h1 := policy.Compute("Title 1", "Body")
		h2 := policy.Compute("Title 2", "Body")
		assert.NotEqual(t, h1, h2)
	})

	t.Run("Component boundary is respected", func(t *testing.T) {
		// "AB" + "C" vs "A" + "BC"
		h1 := policy.Compute("AB", "C")
		h2 := policy.Compute("A", "BC")
		assert.NotEqual(t, h1, h2)
	})
}
