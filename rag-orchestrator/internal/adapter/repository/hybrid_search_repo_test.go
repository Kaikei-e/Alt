package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

type neighborRowItem struct {
	chunkID   uuid.UUID
	articleID string
	score     float32
}

type neighborSearchRows struct {
	pgx.Rows
	all     []neighborRowItem
	visible []neighborRowItem
	i       int
}

func (r *neighborSearchRows) applyQuery(query string, args []interface{}) {
	candidates := append([]neighborRowItem(nil), r.all...)
	limitIdx := strings.Index(query, "LIMIT $3")
	seedFilterIdx := strings.Index(query, "NOT (d.article_id = ANY(")
	if seedFilterIdx >= 0 && (limitIdx < 0 || seedFilterIdx < limitIdx) {
		var seeds []string
		for _, arg := range args {
			if s, ok := arg.([]string); ok {
				seeds = s
				break
			}
		}
		seedSet := make(map[string]struct{}, len(seeds))
		for _, s := range seeds {
			seedSet[s] = struct{}{}
		}
		filtered := candidates[:0]
		for _, item := range candidates {
			if _, isSeed := seedSet[item.articleID]; !isSeed {
				filtered = append(filtered, item)
			}
		}
		candidates = filtered
	}
	if len(args) >= 3 {
		if limit, ok := args[2].(int); ok && limit >= 0 && len(candidates) > limit {
			candidates = candidates[:limit]
		}
	}
	r.visible = candidates
}

func (r *neighborSearchRows) Next() bool {
	if r.i >= len(r.visible) {
		return false
	}
	r.i++
	return true
}

func (r *neighborSearchRows) Scan(dest ...any) error {
	row := r.visible[r.i-1]
	uuidsSeen := 0
	stringsSeen := 0
	for _, d := range dest {
		switch v := d.(type) {
		case *float32:
			*v = row.score
		case *uuid.UUID:
			if uuidsSeen == 0 {
				*v = row.chunkID
			} else {
				*v = uuid.Nil
			}
			uuidsSeen++
		case *int:
			*v = 1
		case *string:
			if stringsSeen == 0 {
				*v = "chunk content"
			} else {
				*v = row.articleID
			}
			stringsSeen++
		case *sql.NullString:
			*v = sql.NullString{String: "value", Valid: true}
		case *time.Time:
			*v = fixedCreatedAt
		}
	}
	return nil
}

func (r *neighborSearchRows) Close()     {}
func (r *neighborSearchRows) Err() error { return nil }

type neighborRecordingTx struct {
	recordingTx
}

func (f *neighborRecordingTx) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	if nr, ok := f.rows.(*neighborSearchRows); ok {
		nr.applyQuery(query, args)
	}
	return f.recordingTx.Query(ctx, query, args...)
}

func TestSearchNeighbors_ExcludesSeedArticlesBeforeLimit(t *testing.T) {
	t.Run("SQL excludes seed article IDs in candidate arms before LIMIT", func(t *testing.T) {
		q := buildSearchNeighborsQuery("english", 60, 4, 5)
		parts := strings.Split(q, "rrf AS")
		require.Len(t, parts, 2)
		vectorPart := strings.Split(parts[0], "text_matches AS")[0]
		textPart := strings.Split(parts[0], "text_matches AS")[1]

		assert.Contains(t, vectorPart, "NOT (d.article_id = ANY($6))")
		assert.Less(t, strings.Index(vectorPart, "NOT (d.article_id = ANY($6))"), strings.Index(vectorPart, "LIMIT $2"))
		assert.Contains(t, textPart, "NOT (d.article_id = ANY($6))")
		assert.Less(t, strings.Index(textPart, "NOT (d.article_id = ANY($6))"), strings.Index(textPart, "LIMIT $2"))
		assert.Contains(t, parts[1], "NOT (d.article_id = ANY($6))")
	})

	t.Run("seed with 4 chunks plus one neighbor article returns the neighbor", func(t *testing.T) {
		repo := NewHybridSearchRepository(nil, 60)
		neighborChunkID := uuid.New()
		rows := &neighborSearchRows{
			all: []neighborRowItem{
				{chunkID: uuid.New(), articleID: "seed-1", score: 0.033},
				{chunkID: uuid.New(), articleID: "seed-1", score: 0.032},
				{chunkID: uuid.New(), articleID: "seed-1", score: 0.031},
				{chunkID: uuid.New(), articleID: "seed-1", score: 0.030},
				{chunkID: neighborChunkID, articleID: "neighbor-1", score: 0.025},
			},
		}
		tx := &neighborRecordingTx{recordingTx: recordingTx{rows: rows}}
		ctx := InjectTx(context.Background(), tx)

		var results []domain.SearchResult
		var err error
		require.NotPanics(t, func() {
			results, err = repo.SearchNeighbors(ctx, []float32{0.1, 0.2}, "seed title", []string{"seed-1"}, 3, chunkRepoTestUserID)
		})
		require.NoError(t, err)
		require.Len(t, tx.args, 1)
		assert.Contains(t, tx.args[0], []string{"seed-1"})
		require.Len(t, results, 1)
		assert.Equal(t, "neighbor-1", results[0].ArticleID)
		assert.Equal(t, neighborChunkID, results[0].Chunk.ID)
	})
}
