package domain_test

import (
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

func chunk(ordinal int, content, hash string) domain.Chunk {
	return domain.Chunk{Ordinal: ordinal, Content: content, Hash: hash}
}

func TestDiffChunks(t *testing.T) {
	// Setup helper chunks
	// We use "A", "B", "C" as content and hash for simplicity.
	cA := chunk(0, "A", "hA")
	cB := chunk(1, "B", "hB")
	cC := chunk(2, "C", "hC")
	cD := chunk(3, "D", "hD")

	cB_prime := chunk(1, "B'", "hB'")

	t.Run("Identity", func(t *testing.T) {
		oldChunks := []domain.Chunk{cA, cB, cC}
		newChunks := []domain.Chunk{cA, cB, cC}

		events := domain.DiffChunks(oldChunks, newChunks)

		assert.Len(t, events, 3)
		assert.Equal(t, domain.ChunkEventUnchanged, events[0].Type)
		assert.Equal(t, domain.ChunkEventUnchanged, events[1].Type)
		assert.Equal(t, domain.ChunkEventUnchanged, events[2].Type)
	})

	t.Run("Update single chunk (same count)", func(t *testing.T) {
		oldChunks := []domain.Chunk{cA, cB, cC}
		newChunks := []domain.Chunk{cA, cB_prime, cC}

		events := domain.DiffChunks(oldChunks, newChunks)

		assert.Len(t, events, 3)

		// A Unchanged
		assert.Equal(t, domain.ChunkEventUnchanged, events[0].Type)
		assert.Equal(t, "A", events[0].NewChunk.Content)

		// B Updated
		assert.Equal(t, domain.ChunkEventUpdated, events[1].Type)
		assert.Equal(t, "B", events[1].OldChunk.Content)
		assert.Equal(t, "B'", events[1].NewChunk.Content)

		// C Unchanged
		assert.Equal(t, domain.ChunkEventUnchanged, events[2].Type)
	})

	t.Run("Add chunk at end", func(t *testing.T) {
		oldChunks := []domain.Chunk{cA, cB}
		newChunks := []domain.Chunk{cA, cB, cC}

		events := domain.DiffChunks(oldChunks, newChunks)

		assert.Len(t, events, 3)
		assert.Equal(t, domain.ChunkEventUnchanged, events[0].Type)
		assert.Equal(t, domain.ChunkEventUnchanged, events[1].Type)
		assert.Equal(t, domain.ChunkEventAdded, events[2].Type)
		assert.Equal(t, "C", events[2].NewChunk.Content)
	})

	t.Run("Delete chunk at end", func(t *testing.T) {
		oldChunks := []domain.Chunk{cA, cB}
		newChunks := []domain.Chunk{cA}

		events := domain.DiffChunks(oldChunks, newChunks)

		assert.Len(t, events, 2)
		assert.Equal(t, domain.ChunkEventUnchanged, events[0].Type)
		assert.Equal(t, domain.ChunkEventDeleted, events[1].Type)
		assert.Equal(t, "B", events[1].OldChunk.Content)
	})

	t.Run("Insert chunk in middle", func(t *testing.T) {
		oldChunks := []domain.Chunk{cA, cC}
		newChunks := []domain.Chunk{cA, cB, cC}

		events := domain.DiffChunks(oldChunks, newChunks)

		assert.Len(t, events, 3)
		assert.Equal(t, domain.ChunkEventUnchanged, events[0].Type)
		// Gap: Old=[], New=[B] -> Added
		assert.Equal(t, domain.ChunkEventAdded, events[1].Type)
		assert.Equal(t, "B", events[1].NewChunk.Content)

		assert.Equal(t, domain.ChunkEventUnchanged, events[2].Type)
	})

	t.Run("Complex Change", func(t *testing.T) {
		// A, B, C -> A, D, C (B replaced by D -> Updated)
		oldChunks := []domain.Chunk{cA, cB, cC}
		newChunks := []domain.Chunk{cA, cD, cC}

		events := domain.DiffChunks(oldChunks, newChunks)
		assert.Len(t, events, 3)
		assert.Equal(t, domain.ChunkEventUpdated, events[1].Type)
		assert.Equal(t, "B", events[1].OldChunk.Content)
		assert.Equal(t, "D", events[1].NewChunk.Content)
	})

	t.Run("Delete and Add (count mismatch)", func(t *testing.T) {
		// A, B -> A, C, D (B replaced by C, D)
		oldChunks := []domain.Chunk{cA, cB}
		newChunks := []domain.Chunk{cA, cC, cD}

		events := domain.DiffChunks(oldChunks, newChunks)
		// A Unchanged
		// B Deleted
		// C Added
		// D Added
		assert.Len(t, events, 4)
		assert.Equal(t, domain.ChunkEventUnchanged, events[0].Type)
		assert.Equal(t, domain.ChunkEventDeleted, events[1].Type)
		assert.Equal(t, "B", events[1].OldChunk.Content)
		assert.Equal(t, domain.ChunkEventAdded, events[2].Type)
		assert.Equal(t, "C", events[2].NewChunk.Content)
		assert.Equal(t, domain.ChunkEventAdded, events[3].Type)
		assert.Equal(t, "D", events[3].NewChunk.Content)
	})
}
