package repository

import (
	"context"
	"database/sql"
	"math"
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/port"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type similarityFakeRow struct {
	seedIndexed       bool
	indexedCandidates int
	articleID         sql.NullString
	similarity        sql.NullFloat64
}

type similarityFakeRows struct {
	pgx.Rows
	items []similarityFakeRow
	idx   int
	err   error
}

func (r *similarityFakeRows) Next() bool {
	if r.idx >= len(r.items) {
		return false
	}
	r.idx++
	return true
}

func (r *similarityFakeRows) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	row := r.items[r.idx-1]
	require := func(ok bool, msg string) {
		if !ok {
			panic(msg)
		}
	}
	require(len(dest) == 4, "expected 4 scan destinations")
	*(dest[0].(*bool)) = row.seedIndexed
	*(dest[1].(*int)) = row.indexedCandidates
	*(dest[2].(*sql.NullString)) = row.articleID
	*(dest[3].(*sql.NullFloat64)) = row.similarity
	return nil
}

func (r *similarityFakeRows) Close()     {}
func (r *similarityFakeRows) Err() error { return nil }

type metadataFakeRow struct {
	articleID string
	title     sql.NullString
	url       sql.NullString
}

type metadataFakeRows struct {
	pgx.Rows
	items []metadataFakeRow
	idx   int
}

func (r *metadataFakeRows) Next() bool {
	if r.idx >= len(r.items) {
		return false
	}
	r.idx++
	return true
}

func (r *metadataFakeRows) Scan(dest ...any) error {
	row := r.items[r.idx-1]
	*(dest[0].(*string)) = row.articleID
	*(dest[1].(*sql.NullString)) = row.title
	*(dest[2].(*sql.NullString)) = row.url
	return nil
}

func (r *metadataFakeRows) Close()     {}
func (r *metadataFakeRows) Err() error { return nil }

func TestNewCandidateSimilarityRepository_ImplementsPorts(t *testing.T) {
	var _ port.CandidateSimilarityRepository = NewCandidateSimilarityRepository(nil)
	var _ port.ArticleMetadataReader = NewCandidateSimilarityRepository(nil)
}

func TestCandidateSimilarityRepository_ValidatesInputsWithoutDB(t *testing.T) {
	repo := NewCandidateSimilarityRepository(nil)
	tx := &recordingTx{rows: &similarityFakeRows{}}
	ctx := InjectTx(context.Background(), tx)

	t.Run("requires non-nil user_id", func(t *testing.T) {
		_, err := repo.ComputeCandidateSimilarities(ctx, "seed-1", []string{"cand-1"}, domain.SimilarityRepFirstChunk, uuid.Nil)
		require.Error(t, err)
		assert.Equal(t, 0, tx.calls)
	})

	t.Run("requires non-empty seed_article_id", func(t *testing.T) {
		_, err := repo.ComputeCandidateSimilarities(ctx, "  ", []string{"cand-1"}, domain.SimilarityRepFirstChunk, chunkRepoTestUserID)
		require.Error(t, err)
		assert.Equal(t, 0, tx.calls)
	})

	t.Run("empty candidates skips DB", func(t *testing.T) {
		res, err := repo.ComputeCandidateSimilarities(ctx, "seed-1", nil, domain.SimilarityRepChunkMean, chunkRepoTestUserID)
		require.NoError(t, err)
		assert.Empty(t, res.Results)
		assert.Equal(t, 0, res.CandidatesWithoutEmbedding)
		assert.Equal(t, 0, tx.calls)
	})

	t.Run("unsupported representation returns error without DB", func(t *testing.T) {
		_, err := repo.ComputeCandidateSimilarities(ctx, "seed-1", []string{"cand-1"}, domain.SimilarityRepresentation("unknown"), chunkRepoTestUserID)
		require.Error(t, err)
		assert.Equal(t, 0, tx.calls)
	})
}

func TestCandidateSimilarityRepository_SQLStructurePerRepresentation(t *testing.T) {
	tests := []struct {
		name           string
		rep            domain.SimilarityRepresentation
		mustContain    []string
		mustNotContain []string
	}{
		{
			name: "first_chunk uses ordinal ASC LIMIT 1 per current owned version",
			rep:  domain.SimilarityRepFirstChunk,
			mustContain: []string{
				"d.current_version_id = c.version_id",
				"d.user_id = $3",
				"d.article_id = ANY($2)",
				"d.article_id <> $1",
				"c.embedding IS NOT NULL",
				"ORDER BY c.ordinal ASC",
				"LIMIT 1",
				"LATERAL",
				"<=>",
				"ORDER BY s.similarity DESC NULLS LAST, s.article_id ASC",
			},
		},
		{
			name: "chunk_mean uses AVG(c.embedding) grouped by article_id",
			rep:  domain.SimilarityRepChunkMean,
			mustContain: []string{
				"AVG(c.embedding)",
				"HAVING COUNT(c.embedding) > 0",
				"GROUP BY d.article_id",
				"d.current_version_id = c.version_id",
				"d.user_id = $3",
				"d.article_id = ANY($2)",
				"d.article_id <> $1",
				"c.embedding IS NOT NULL",
				"<=>",
				"ORDER BY s.similarity DESC NULLS LAST, s.article_id ASC",
			},
		},
		{
			name: "nearest_chunk_pair uses MAX similarity across chunk pairs",
			rep:  domain.SimilarityRepNearestChunkPair,
			mustContain: []string{
				"MAX(1.0 - (sc.embedding <=> cc.embedding))",
				"CROSS JOIN cand_chunks cc",
				"GROUP BY cc.article_id",
				"d.current_version_id = c.version_id",
				"d.user_id = $3",
				"d.article_id = ANY($2)",
				"d.article_id <> $1",
				"c.embedding IS NOT NULL",
				"ORDER BY s.similarity DESC NULLS LAST, s.article_id ASC",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewCandidateSimilarityRepository(nil)
			tx := &recordingTx{rows: &similarityFakeRows{
				items: []similarityFakeRow{
					{
						seedIndexed:       true,
						indexedCandidates: 2,
						articleID:         sql.NullString{String: "cand-1", Valid: true},
						similarity:        sql.NullFloat64{Float64: 0.91, Valid: true},
					},
					{
						seedIndexed:       true,
						indexedCandidates: 2,
						articleID:         sql.NullString{String: "cand-2", Valid: true},
						similarity:        sql.NullFloat64{Float64: 0.72, Valid: true},
					},
				},
			}}
			ctx := InjectTx(context.Background(), tx)

			res, err := repo.ComputeCandidateSimilarities(ctx, "seed-1", []string{"cand-1", "cand-2", "cand-3"}, tc.rep, chunkRepoTestUserID)
			require.NoError(t, err)
			assert.Equal(t, 1, tx.calls, "must execute in a single SQL round-trip")
			require.Len(t, tx.queries, 1)

			q := tx.queries[0]
			for _, token := range tc.mustContain {
				assert.Contains(t, q, token)
			}
			for _, token := range tc.mustNotContain {
				assert.NotContains(t, q, token)
			}

			require.Len(t, tx.args, 1)
			assert.Equal(t, "seed-1", tx.args[0][0])
			assert.Equal(t, []string{"cand-1", "cand-2", "cand-3"}, tx.args[0][1])
			assert.Equal(t, chunkRepoTestUserID, tx.args[0][2])

			assert.True(t, res.SeedIndexed)
			assert.Equal(t, 1, res.CandidatesWithoutEmbedding)
			require.Len(t, res.Results, 2)
			assert.Equal(t, "cand-1", res.Results[0].ArticleID)
			assert.InDelta(t, 0.91, res.Results[0].Similarity, 1e-9)
			assert.Equal(t, "cand-2", res.Results[1].ArticleID)
			assert.InDelta(t, 0.72, res.Results[1].Similarity, 1e-9)
		})
	}
}

func TestCandidateSimilarityRepository_SeedWithoutEmbeddingReturnsEmptyAndFlag(t *testing.T) {
	repo := NewCandidateSimilarityRepository(nil)
	tx := &recordingTx{rows: &similarityFakeRows{
		items: []similarityFakeRow{
			{
				seedIndexed:       false,
				indexedCandidates: 2,
				articleID:         sql.NullString{Valid: false},
				similarity:        sql.NullFloat64{Valid: false},
			},
		},
	}}
	ctx := InjectTx(context.Background(), tx)

	res, err := repo.ComputeCandidateSimilarities(ctx, "seed-unindexed", []string{"cand-1", "cand-2", "cand-3"}, domain.SimilarityRepChunkMean, chunkRepoTestUserID)
	require.NoError(t, err)
	assert.False(t, res.SeedIndexed)
	assert.Empty(t, res.Results)
	assert.Equal(t, 1, res.CandidatesWithoutEmbedding)
}

func TestCandidateSimilarityRepository_SkipsNaNSimilarityAndCountsMissing(t *testing.T) {
	repo := NewCandidateSimilarityRepository(nil)
	tx := &recordingTx{rows: &similarityFakeRows{
		items: []similarityFakeRow{
			{
				seedIndexed:       true,
				indexedCandidates: 2,
				articleID:         sql.NullString{String: "cand-nan", Valid: true},
				similarity:        sql.NullFloat64{Float64: math.NaN(), Valid: true},
			},
			{
				seedIndexed:       true,
				indexedCandidates: 2,
				articleID:         sql.NullString{String: "cand-ok", Valid: true},
				similarity:        sql.NullFloat64{Float64: 0.85, Valid: true},
			},
		},
	}}
	ctx := InjectTx(context.Background(), tx)

	res, err := repo.ComputeCandidateSimilarities(ctx, "seed-1", []string{"cand-nan", "cand-ok"}, domain.SimilarityRepFirstChunk, chunkRepoTestUserID)
	require.NoError(t, err)
	assert.True(t, res.SeedIndexed)
	require.Len(t, res.Results, 1)
	assert.Equal(t, "cand-ok", res.Results[0].ArticleID)
	assert.Equal(t, 1, res.CandidatesWithoutEmbedding)
}

func TestCandidateSimilarityRepository_ScanErrorPropagates(t *testing.T) {
	repo := NewCandidateSimilarityRepository(nil)
	tx := &recordingTx{rows: &similarityFakeRows{
		items: []similarityFakeRow{{}},
		err:   errScanBoom,
	}}
	ctx := InjectTx(context.Background(), tx)

	res, err := repo.ComputeCandidateSimilarities(ctx, "seed-1", []string{"cand-1"}, domain.SimilarityRepFirstChunk, chunkRepoTestUserID)
	require.ErrorIs(t, err, errScanBoom)
	assert.Nil(t, res)
}

func TestCandidateSimilarityRepository_GetArticleMetadataByIDs(t *testing.T) {
	repo := NewCandidateSimilarityRepository(nil)
	tx := &recordingTx{rows: &metadataFakeRows{
		items: []metadataFakeRow{
			{
				articleID: "art-1",
				title:     sql.NullString{String: "Title One", Valid: true},
				url:       sql.NullString{String: "https://example.com/1", Valid: true},
			},
			{
				articleID: "art-2",
				title:     sql.NullString{Valid: false},
				url:       sql.NullString{Valid: false},
			},
		},
	}}
	ctx := InjectTx(context.Background(), tx)

	meta, err := repo.GetArticleMetadataByIDs(ctx, []string{"art-1", "art-2"}, chunkRepoTestUserID)
	require.NoError(t, err)
	assert.Equal(t, 1, tx.calls)
	require.Len(t, tx.queries, 1)
	q := tx.queries[0]
	assert.Contains(t, q, "d.current_version_id = v.id")
	assert.Contains(t, q, "d.user_id = $2")
	assert.Contains(t, q, "d.article_id = ANY($1)")
	assert.Equal(t, "Title One", meta["art-1"].Title)
	assert.Equal(t, "https://example.com/1", meta["art-1"].URL)
	assert.Empty(t, meta["art-2"].Title)
	assert.Empty(t, meta["art-2"].URL)

	t.Run("empty ids skips DB", func(t *testing.T) {
		txEmpty := &recordingTx{rows: &metadataFakeRows{}}
		emptyMeta, err := repo.GetArticleMetadataByIDs(InjectTx(context.Background(), txEmpty), nil, chunkRepoTestUserID)
		require.NoError(t, err)
		assert.Empty(t, emptyMeta)
		assert.Equal(t, 0, txEmpty.calls)
	})

	t.Run("nil user_id returns error", func(t *testing.T) {
		_, err := repo.GetArticleMetadataByIDs(ctx, []string{"art-1"}, uuid.Nil)
		require.Error(t, err)
	})
}

func TestCandidateSimilarityRepository_SQLUsesExpectedOrder(t *testing.T) {
	// Ensure no unguarded strings.Contains mistakes in query builders
	for _, rep := range domain.AllSimilarityRepresentations {
		q, err := buildCandidateSimilarityQuery(rep)
		require.NoError(t, err)
		assert.Contains(t, q, "WITH ")
		assert.Contains(t, q, "d.user_id = $3")
		assert.Contains(t, q, "d.current_version_id = c.version_id")
	}
}
