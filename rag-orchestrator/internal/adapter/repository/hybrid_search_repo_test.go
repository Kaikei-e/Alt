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

func assertCandidateArmUsesOwnerSemiJoinBeforeLimit(t *testing.T, arm, ownerPlaceholder, limitPlaceholder string) {
	t.Helper()
	assert.NotContains(t, arm, "rag_document_versions")
	assert.Contains(t, arm, "EXISTS")
	assert.Contains(t, arm, "FROM rag_documents d")
	assert.Contains(t, arm, "d.current_version_id = c.version_id")
	ownerFilter := "d.user_id = " + ownerPlaceholder
	limitClause := "LIMIT " + limitPlaceholder
	assert.Contains(t, arm, ownerFilter)
	assert.Contains(t, arm, limitClause)
	assert.Less(t, strings.Index(arm, "EXISTS"), strings.Index(arm, ownerFilter))
	assert.Less(t, strings.Index(arm, "EXISTS"), strings.Index(arm, limitClause))
	assert.Less(t, strings.Index(arm, "d.current_version_id = c.version_id"), strings.Index(arm, limitClause))
	assert.Less(t, strings.Index(arm, ownerFilter), strings.Index(arm, limitClause))
}

func assertVectorArmMaterializesAndResortsCandidates(t *testing.T, vectorPart, vectorPlaceholder, ownerPlaceholder, limitPlaceholder string) {
	t.Helper()
	assert.Contains(t, vectorPart, "vector_candidates AS MATERIALIZED")
	assert.Contains(t, vectorPart, "vector_matches AS")
	subparts := strings.SplitN(vectorPart, "vector_matches AS", 2)
	require.Len(t, subparts, 2, "must have vector_candidates and vector_matches CTEs")
	vectorCandidatesCTE := subparts[0]
	vectorMatchesCTE := subparts[1]

	assertCandidateArmUsesOwnerSemiJoinBeforeLimit(t, vectorCandidatesCTE, ownerPlaceholder, limitPlaceholder)
	orderBy := "ORDER BY c.embedding <=> " + vectorPlaceholder
	limitClause := "LIMIT " + limitPlaceholder
	assert.Contains(t, vectorCandidatesCTE, orderBy)
	assert.Less(t, strings.Index(vectorCandidatesCTE, orderBy), strings.Index(vectorCandidatesCTE, limitClause))
	assert.Contains(t, vectorMatchesCTE, "rank() OVER (ORDER BY distance + 0) AS rank")
	assert.Contains(t, vectorMatchesCTE, "FROM vector_candidates")
}

func assertOuterMetadataRejoin(t *testing.T, outerPart, ownerPlaceholder string) {
	t.Helper()
	assert.Contains(t, outerPart, "JOIN rag_documents d ON v.document_id = d.id")
	assert.Contains(t, outerPart, "d.current_version_id = v.id")
	assert.Contains(t, outerPart, "d.user_id = "+ownerPlaceholder)
}

func TestHybridSearch_CandidatePoolFiltersByOwnerWithinCandidateLimit(t *testing.T) {
	q := buildHybridSearchQuery("english", 60)

	// Ensure both vector_candidates and text_matches filter current_version_id and user_id ($5) before candidate limit ($3)
	parts := strings.Split(q, "rrf AS")
	require.Len(t, parts, 2, "must contain candidate CTEs and rrf CTE")
	candidatesPart := parts[0]

	vectorPart := strings.Split(candidatesPart, "text_matches AS")[0]
	assertVectorArmMaterializesAndResortsCandidates(t, vectorPart, "$1", "$5", "$3")

	textPart := strings.Split(candidatesPart, "text_matches AS")[1]
	assertCandidateArmUsesOwnerSemiJoinBeforeLimit(t, textPart, "$5", "$3")

	assertOuterMetadataRejoin(t, parts[1], "$5")
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
		assertVectorArmMaterializesAndResortsCandidates(t, vectorPart, "$4", "$5", "$2")

		textPart := strings.Split(candidatesPart, "text_matches AS")[1]
		assertCandidateArmUsesOwnerSemiJoinBeforeLimit(t, textPart, "$5", "$2")

		assertOuterMetadataRejoin(t, parts[1], "$5")
	})

	t.Run("text only arm", func(t *testing.T) {
		// vectorArmArg = 0, userArgIdx = 4
		q := buildSearchNeighborsQuery("english", 60, 0, 4)

		parts := strings.Split(q, "rrf AS")
		require.Len(t, parts, 2, "must contain candidate CTEs and rrf CTE")
		candidatesPart := parts[0]

		assert.NotContains(t, candidatesPart, "vector_candidates AS")
		assert.NotContains(t, candidatesPart, "vector_matches AS")

		textPart := candidatesPart
		assertCandidateArmUsesOwnerSemiJoinBeforeLimit(t, textPart, "$4", "$2")

		assertOuterMetadataRejoin(t, parts[1], "$4")
	})
}

func TestHybridSearch_VectorArmsExcludeNullEmbeddingsOnly(t *testing.T) {
	t.Run("HybridSearch excludes NULL embeddings in vector_candidates only", func(t *testing.T) {
		q := buildHybridSearchQuery("english", 60)
		parts := strings.Split(q, "rrf AS")
		require.Len(t, parts, 2)
		vectorPart := strings.Split(parts[0], "text_matches AS")[0]
		textPart := strings.Split(parts[0], "text_matches AS")[1]

		assert.Contains(t, vectorPart, "c.embedding IS NOT NULL")
		assert.Less(t, strings.Index(vectorPart, "c.embedding IS NOT NULL"), strings.Index(vectorPart, "ORDER BY c.embedding <=> $1"))
		assert.NotContains(t, textPart, "c.embedding IS NOT NULL")
	})

	t.Run("SearchNeighbors excludes NULL embeddings in vector_candidates only", func(t *testing.T) {
		q := buildSearchNeighborsQuery("english", 60, 4, 5)
		parts := strings.Split(q, "rrf AS")
		require.Len(t, parts, 2)
		vectorPart := strings.Split(parts[0], "text_matches AS")[0]
		textPart := strings.Split(parts[0], "text_matches AS")[1]

		assert.Contains(t, vectorPart, "c.embedding IS NOT NULL")
		assert.Less(t, strings.Index(vectorPart, "c.embedding IS NOT NULL"), strings.Index(vectorPart, "ORDER BY c.embedding <=> $4"))
		assert.NotContains(t, textPart, "c.embedding IS NOT NULL")
	})
}
