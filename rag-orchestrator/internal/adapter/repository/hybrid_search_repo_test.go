package repository

import (
	"strings"
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHybridSearchRepository_ImplementsInterface(t *testing.T) {
	// Verify that the constructor returns the correct interface.
	// Cannot call methods without a real DB, but we verify the type.
	var _ domain.HybridSearcher = NewHybridSearchRepository(nil, 60)
}

func TestNewHybridSearchRepository_DefaultRRFK(t *testing.T) {
	repo := NewHybridSearchRepository(nil, 0)
	// With rrfK=0, should default to 60
	assert.NotNil(t, repo)

	concrete := repo.(*hybridSearchRepository)
	assert.Equal(t, 60, concrete.rrfK)
}

func TestNewHybridSearchRepository_CustomRRFK(t *testing.T) {
	repo := NewHybridSearchRepository(nil, 30)
	concrete := repo.(*hybridSearchRepository)
	assert.Equal(t, 30, concrete.rrfK)
}

func TestHybridSearch_CandidatePoolFiltersByOwnerWithinCandidateLimit(t *testing.T) {
	q := buildHybridSearchQuery("english", 60)

	// Ensure both vector_matches and text_matches filter current_version_id and user_id ($5) before candidate limit ($3)
	parts := strings.Split(q, "rrf AS")
	require.Len(t, parts, 2, "must contain candidate CTEs and rrf CTE")
	candidatesPart := parts[0]

	vectorPart := strings.Split(candidatesPart, "text_matches AS")[0]
	assert.Contains(t, vectorPart, "JOIN rag_document_versions v ON c.version_id = v.id")
	assert.Contains(t, vectorPart, "JOIN rag_documents d ON v.document_id = d.id")
	assert.Contains(t, vectorPart, "d.current_version_id = v.id")
	assert.Contains(t, vectorPart, "d.user_id = $5")
	assert.Contains(t, vectorPart, "LIMIT $3")

	textPart := strings.Split(candidatesPart, "text_matches AS")[1]
	assert.Contains(t, textPart, "JOIN rag_document_versions v ON c.version_id = v.id")
	assert.Contains(t, textPart, "JOIN rag_documents d ON v.document_id = d.id")
	assert.Contains(t, textPart, "d.current_version_id = v.id")
	assert.Contains(t, textPart, "d.user_id = $5")
	assert.Contains(t, textPart, "LIMIT $3")
}

func TestSearchNeighbors_CandidatePoolFiltersByOwnerWithinCandidateLimit(t *testing.T) {
	t.Run("vector and text arms", func(t *testing.T) {
		// vectorArmArg = 4, userArgIdx = 5
		q := buildSearchNeighborsQuery("english", 60, 4, 5)

		parts := strings.Split(q, "rrf AS")
		require.Len(t, parts, 2, "must contain candidate CTEs and rrf CTE")
		candidatesPart := parts[0]

		require.Contains(t, candidatesPart, "vector_matches AS")
		vectorPart := strings.Split(candidatesPart, "text_matches AS")[0]
		assert.Contains(t, vectorPart, "JOIN rag_document_versions v ON c.version_id = v.id")
		assert.Contains(t, vectorPart, "JOIN rag_documents d ON v.document_id = d.id")
		assert.Contains(t, vectorPart, "d.current_version_id = v.id")
		assert.Contains(t, vectorPart, "d.user_id = $5")
		assert.Contains(t, vectorPart, "LIMIT $2")

		textPart := strings.Split(candidatesPart, "text_matches AS")[1]
		assert.Contains(t, textPart, "JOIN rag_document_versions v ON c.version_id = v.id")
		assert.Contains(t, textPart, "JOIN rag_documents d ON v.document_id = d.id")
		assert.Contains(t, textPart, "d.current_version_id = v.id")
		assert.Contains(t, textPart, "d.user_id = $5")
		assert.Contains(t, textPart, "LIMIT $2")
	})

	t.Run("text only arm", func(t *testing.T) {
		// vectorArmArg = 0, userArgIdx = 4
		q := buildSearchNeighborsQuery("english", 60, 0, 4)

		parts := strings.Split(q, "rrf AS")
		require.Len(t, parts, 2, "must contain candidate CTEs and rrf CTE")
		candidatesPart := parts[0]

		assert.NotContains(t, candidatesPart, "vector_matches AS")

		textPart := candidatesPart
		assert.Contains(t, textPart, "JOIN rag_document_versions v ON c.version_id = v.id")
		assert.Contains(t, textPart, "JOIN rag_documents d ON v.document_id = d.id")
		assert.Contains(t, textPart, "d.current_version_id = v.id")
		assert.Contains(t, textPart, "d.user_id = $4")
		assert.Contains(t, textPart, "LIMIT $2")
	})
}
