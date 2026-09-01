package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedCreatedAt stands in for a real created_at column value in the fake
// rows below. It carries no meaning beyond "some parseable time.Time".
var fixedCreatedAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

// nullEmbeddingRow is a single-row pgx.Rows fake that reproduces exactly
// what real pgx does when a SQL NULL is decoded into a non-nullable
// pgvector.Vector destination: pgx's baseRows.Scan (rows.go) calls
// scanPlans[i].Scan(values[i], dst) with values[i] == nil and no NULL
// guard, and the pgvector-go/pgx codec forwards that nil straight into
// Vector.DecodeBinary — which does buf[0:2] on a zero-capacity slice and
// panics. Scan below drives the real, unmodified pgvector-go dependency
// through that exact call shape, so the panic (or its absence, once the
// embedding column stops being scanned) is genuine rather than mocked.
type nullEmbeddingRow struct {
	id, versionID uuid.UUID
	served        bool
}

func (r *nullEmbeddingRow) Next() bool {
	if r.served {
		return false
	}
	r.served = true
	return true
}

func (r *nullEmbeddingRow) Scan(dest ...any) error {
	for i, d := range dest {
		switch v := d.(type) {
		case *uuid.UUID:
			if i == 0 {
				*v = r.id
			} else {
				*v = r.versionID
			}
		case *int:
			*v = 0
		case *string:
			*v = "chunk content"
		case *pgvector.Vector:
			// Column value is SQL NULL: reproduce pgx's real NULL decode path.
			return v.DecodeBinary(nil)
		}
	}
	return nil
}

func (r *nullEmbeddingRow) Close()                                       {}
func (r *nullEmbeddingRow) Err() error                                   { return nil }
func (r *nullEmbeddingRow) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *nullEmbeddingRow) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *nullEmbeddingRow) Values() ([]any, error)                       { return nil, nil }
func (r *nullEmbeddingRow) RawValues() [][]byte                          { return nil }
func (r *nullEmbeddingRow) Conn() *pgx.Conn                              { return nil }

// fakeTxQueryOnly is a minimal pgx.Tx double, following the same pattern as
// fakeTx in postgres_tx_test.go: embed pgx.Tx so every unused method panics
// loudly on a nil-interface call, and override only what the code under
// test actually calls (Query).
type fakeTxQueryOnly struct {
	pgx.Tx
	rows pgx.Rows
}

func (f *fakeTxQueryOnly) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return f.rows, nil
}

// recordingTx is a pgx.Tx double that counts Query calls and keeps the args of
// each, so a test can assert both how many round-trips a repository method
// costs and what candidate pool it asked the database for.
type recordingTx struct {
	pgx.Tx
	calls int
	args  [][]interface{}
	rows  pgx.Rows
}

func (f *recordingTx) Query(_ context.Context, _ string, args ...interface{}) (pgx.Rows, error) {
	f.calls++
	f.args = append(f.args, args)
	return f.rows, nil
}

// enrichedChunkRow is a single-row pgx.Rows fake for Search()'s and
// SearchWithinArticles()'s query. Unlike nullEmbeddingRow
// (which relies on a fixed, known dest order), it assigns destinations by
// Go type with an occurrence counter per type, so it tolerates both the
// fixed column list (no embedding) and the pre-fix column list (embedding
// inserted after content) without caring which one the caller built:
// inserting one extra *pgvector.Vector column does not reorder the other
// typed destinations relative to each other. Like nullEmbeddingRow, a SQL
// NULL embedding is reproduced by driving the real pgvector-go codec's
// DecodeBinary(nil) path, which panics — so this fake reproduces the
// actual panic mechanism rather than asserting behavior in the abstract.
type enrichedChunkRow struct {
	pgx.Rows
	id, versionID uuid.UUID
	served        bool
}

func (r *enrichedChunkRow) Next() bool {
	if r.served {
		return false
	}
	r.served = true
	return true
}

func (r *enrichedChunkRow) Scan(dest ...any) error {
	uuidsSeen := 0
	intsSeen := 0
	stringsSeen := 0
	for _, d := range dest {
		switch v := d.(type) {
		case *uuid.UUID:
			if uuidsSeen == 0 {
				*v = r.id
			} else {
				*v = r.versionID
			}
			uuidsSeen++
		case *int:
			if intsSeen == 0 {
				*v = 3 // ordinal
			} else {
				*v = 1 // version_number
			}
			intsSeen++
		case *string:
			if stringsSeen == 0 {
				*v = "chunk content" // content
			} else {
				*v = "article-1" // article_id
			}
			stringsSeen++
		case *sql.NullString:
			*v = sql.NullString{String: "value", Valid: true}
		case *time.Time:
			*v = fixedCreatedAt
		case *float32:
			*v = 0.1
		case *pgvector.Vector:
			// Column value is SQL NULL: reproduce pgx's real NULL decode path.
			return v.DecodeBinary(nil)
		}
	}
	return nil
}

func (r *enrichedChunkRow) Close()     {}
func (r *enrichedChunkRow) Err() error { return nil }

// TestGetChunksByVersionID_NullEmbedding_DoesNotPanic is the RED case for
// rag-null-embedding-panic: migration 20260802120000 widened
// rag_chunks.embedding to vector(1024) via `USING NULL::vector(1024)`, so
// every existing chunk row now has a SQL NULL embedding. GetChunksByVersionID
// is only ever used to compute a chunk diff (by Ordinal/Content/ID) or to
// build retrieval context text — no caller reads Embedding from its result —
// yet it scans the embedding column into a non-nullable pgvector.Vector,
// which panics on NULL. It must stop decoding that column.
func TestGetChunksByVersionID_NullEmbedding_DoesNotPanic(t *testing.T) {
	repo := NewRagChunkRepository(nil)

	versionID := uuid.New()
	row := &nullEmbeddingRow{id: uuid.New(), versionID: versionID}
	ctx := InjectTx(context.Background(), &fakeTxQueryOnly{rows: row})

	var chunks []domain.RagChunk
	var err error
	require.NotPanics(t, func() {
		chunks, err = repo.GetChunksByVersionID(ctx, versionID)
	}, "GetChunksByVersionID must not panic when embedding is SQL NULL")

	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, "chunk content", chunks[0].Content)
	assert.Equal(t, versionID, chunks[0].VersionID)
}

// TestSearch_NullEmbedding_DoesNotPanic is the RED case for the first
// defect the reviewer confirmed in rag-null-embedding-panic: Search()'s
// enrichment query scanned c.embedding into the same non-nullable
// domain.RagChunk.Embedding as GetChunksByVersionID did, and Search() runs
// inside an errgroup goroutine (embed_and_search.go's default branch), so
// the panic crashes the process rather than surfacing as a recovered 500.
// The query does not filter on "embedding IS NOT NULL", so a NULL-embedding
// chunk reaches Scan.
func TestSearch_NullEmbedding_DoesNotPanic(t *testing.T) {
	repo := NewRagChunkRepository(nil)

	chunkID := uuid.New()
	versionID := uuid.New()
	tx := &recordingTx{rows: &enrichedChunkRow{id: chunkID, versionID: versionID}}
	ctx := InjectTx(context.Background(), tx)

	var results []domain.SearchResult
	var err error
	require.NotPanics(t, func() {
		results, err = repo.Search(ctx, []float32{0.1, 0.2}, 5)
	}, "Search must not panic when a chunk's embedding is SQL NULL")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "chunk content", results[0].Chunk.Content)
	assert.Equal(t, "article-1", results[0].ArticleID)
	// Score must be derived from the scanned distance (1.0 - 0.1), not a
	// zero-value fallback: losing it flattens every result to Score 1.0 and
	// destroys ranking without failing any scan.
	assert.InDelta(t, 0.9, results[0].Score, 1e-6)
}

// TestSearch_IsOneRoundTrip pins the cost of a retrieval. Search used to run a
// candidate query and an enrichment query back to back, and Stage 3 fans it out
// over up to nine expanded queries — eighteen round-trips for one question.
func TestSearch_IsOneRoundTrip(t *testing.T) {
	repo := NewRagChunkRepository(nil)

	chunkID := uuid.New()
	tx := &recordingTx{rows: &enrichedChunkRow{id: chunkID, versionID: uuid.New()}}
	ctx := InjectTx(context.Background(), tx)

	_, err := repo.Search(ctx, []float32{0.1, 0.2}, 50)
	require.NoError(t, err)

	assert.Equal(t, 1, tx.calls, "one vector search must cost one database round-trip")
}

// TestSearch_OverfetchIsRightSized: the candidate pool only has to absorb rows
// dropped for belonging to a superseded document version, which is a rare state,
// not a third of the corpus. The old 3x/500 pool made every HNSW scan read far
// more than the query could use.
func TestSearch_OverfetchIsRightSized(t *testing.T) {
	repo := NewRagChunkRepository(nil)

	tx := &recordingTx{rows: &enrichedChunkRow{id: uuid.New(), versionID: uuid.New()}}
	ctx := InjectTx(context.Background(), tx)

	_, err := repo.Search(ctx, []float32{0.1, 0.2}, 50)
	require.NoError(t, err)

	require.Len(t, tx.args, 1)
	args := tx.args[0]
	require.Len(t, args, 3, "query vector, candidate pool size, final limit")
	assert.Equal(t, searchOverfetchMultiplier*50, args[1])
	assert.Equal(t, 50, args[2])
}

// TestSearch_OverfetchIsCapped keeps a large SearchLimit from asking pgvector
// for an unbounded candidate pool.
func TestSearch_OverfetchIsCapped(t *testing.T) {
	repo := NewRagChunkRepository(nil)

	tx := &recordingTx{rows: &enrichedChunkRow{id: uuid.New(), versionID: uuid.New()}}
	ctx := InjectTx(context.Background(), tx)

	_, err := repo.Search(ctx, []float32{0.1, 0.2}, 5000)
	require.NoError(t, err)

	assert.Equal(t, searchOverfetchCap, tx.args[0][1])
}

// TestSearch_TagsScoresAsVector: a cosine similarity is not a cross-encoder
// score, and the quality gate can only know that if the result says so.
func TestSearch_TagsScoresAsVector(t *testing.T) {
	repo := NewRagChunkRepository(nil)

	tx := &recordingTx{rows: &enrichedChunkRow{id: uuid.New(), versionID: uuid.New()}}
	ctx := InjectTx(context.Background(), tx)

	results, err := repo.Search(ctx, []float32{0.1, 0.2}, 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, domain.ScoreKindVector, results[0].ScoreKind)
}

func TestSearchWithinArticles_TagsScoresAsVector(t *testing.T) {
	repo := NewRagChunkRepository(nil)

	ctx := InjectTx(context.Background(), &fakeTxQueryOnly{rows: &enrichedChunkRow{id: uuid.New(), versionID: uuid.New()}})

	results, err := repo.SearchWithinArticles(ctx, []float32{0.1, 0.2}, []string{"article-1"}, 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, domain.ScoreKindVector, results[0].ScoreKind)
}

// TestSearchWithinArticles_NullEmbedding_DoesNotPanic is the RED case for
// the same defect in SearchWithinArticles(): its own comment says
// filtering to a small article subset makes the missing HNSW usage "ok",
// but says nothing about NULL embeddings, and the query has no
// "embedding IS NOT NULL" guard. It scanned c.embedding into the same
// non-nullable field, reached from goroutine F in embed_and_search.go via
// the hasCandidateArticles branch — also inside an errgroup goroutine.
func TestSearchWithinArticles_NullEmbedding_DoesNotPanic(t *testing.T) {
	repo := NewRagChunkRepository(nil)

	chunkID := uuid.New()
	versionID := uuid.New()
	row := &enrichedChunkRow{id: chunkID, versionID: versionID}
	ctx := InjectTx(context.Background(), &fakeTxQueryOnly{rows: row})

	var results []domain.SearchResult
	var err error
	require.NotPanics(t, func() {
		results, err = repo.SearchWithinArticles(ctx, []float32{0.1, 0.2}, []string{"article-1"}, 5)
	}, "SearchWithinArticles must not panic when a chunk's embedding is SQL NULL")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "chunk content", results[0].Chunk.Content)
	assert.Equal(t, "article-1", results[0].ArticleID)
	// Score must come from the scanned distance column (1.0 - 0.1). Dropping
	// the &distance destination leaves distance at its zero value, silently
	// reporting every chunk as a perfect match.
	assert.InDelta(t, 0.9, results[0].Score, 1e-6)
}

// multiEnrichedRows is a multi-row pgx.Rows fake for the single enrichment
// query: one (chunk, distance) pair per row, served in slice order. The real
// query carries its own ORDER BY on the candidate distance, so the order the
// rows arrive in is the order the caller must keep.
type multiEnrichedRows struct {
	pgx.Rows
	ids       []uuid.UUID
	distances []float32
	i         int
}

func (r *multiEnrichedRows) Next() bool {
	if r.i >= len(r.ids) {
		return false
	}
	r.i++
	return true
}

func (r *multiEnrichedRows) Scan(dest ...any) error {
	uuidsSeen := 0
	for _, d := range dest {
		switch v := d.(type) {
		case *uuid.UUID:
			if uuidsSeen == 0 {
				*v = r.ids[r.i-1]
			} else {
				*v = uuid.Nil
			}
			uuidsSeen++
		case *int:
			*v = 0
		case *string:
			*v = "content"
		case *sql.NullString:
			*v = sql.NullString{String: "value", Valid: true}
		case *time.Time:
			*v = fixedCreatedAt
		case *float32:
			*v = r.distances[r.i-1]
		}
	}
	return nil
}

func (r *multiEnrichedRows) Close()     {}
func (r *multiEnrichedRows) Err() error { return nil }

// errScanBoom stands in for any driver-level row decode failure (type
// mismatch, connection loss mid-row).
var errScanBoom = errors.New("scan boom")

// erroringRow is a single-row pgx.Rows fake whose Scan always fails.
type erroringRow struct {
	pgx.Rows
	served bool
}

func (r *erroringRow) Next() bool {
	if r.served {
		return false
	}
	r.served = true
	return true
}

func (r *erroringRow) Scan(dest ...any) error { return errScanBoom }
func (r *erroringRow) Close()                 {}
func (r *erroringRow) Err() error             { return nil }

// TestGetChunksByVersionID_ScanErrorPropagates guards the error path the
// NULL-embedding fakes cannot reach (their Scan either panics or succeeds):
// a row that fails to decode must surface as an error, not as a silently
// empty chunk list — an empty list here makes the chunk diff treat the
// whole article as brand new and rewrite every chunk.
func TestGetChunksByVersionID_ScanErrorPropagates(t *testing.T) {
	repo := NewRagChunkRepository(nil)
	ctx := InjectTx(context.Background(), &fakeTxQueryOnly{rows: &erroringRow{}})

	chunks, err := repo.GetChunksByVersionID(ctx, uuid.New())

	require.ErrorIs(t, err, errScanBoom)
	assert.Nil(t, chunks)
}

// TestSearch_ScoresEachRowFromItsOwnDistance pins the observable ranking
// contract: every result's Score is 1.0 minus that row's own distance, and the
// database's ordering (ORDER BY on the candidate distance, LIMIT on the final
// count) is carried through untouched. Flattening the per-row distance to a
// zero value destroys ranking without failing any scan.
func TestSearch_ScoresEachRowFromItsOwnDistance(t *testing.T) {
	repo := NewRagChunkRepository(nil)

	idA, idB, idC := uuid.New(), uuid.New(), uuid.New()
	tx := &recordingTx{rows: &multiEnrichedRows{
		ids:       []uuid.UUID{idA, idC, idB},
		distances: []float32{0.1, 0.2, 0.3},
	}}
	ctx := InjectTx(context.Background(), tx)

	results, err := repo.Search(ctx, []float32{0.1, 0.2}, 3)

	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, idA, results[0].Chunk.ID)
	assert.Equal(t, idC, results[1].Chunk.ID)
	assert.InDelta(t, 0.9, results[0].Score, 1e-6)
	assert.InDelta(t, 0.8, results[1].Score, 1e-6)
	assert.InDelta(t, 0.7, results[2].Score, 1e-6)
}
