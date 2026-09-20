package repository

import (
	"context"
	"database/sql"
	"fmt"
	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

type ragChunkRepository struct {
	pool *pgxpool.Pool
}

// NewRagChunkRepository creates a new RagChunkRepository.
func NewRagChunkRepository(pool *pgxpool.Pool) domain.RagChunkRepository {
	return &ragChunkRepository{pool: pool}
}

type dbExecutor interface {
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}

func (r *ragChunkRepository) getExecutor(ctx context.Context) dbExecutor {
	tx := ExtractTx(ctx)
	if tx != nil {
		return tx
	}
	return r.pool
}

func (r *ragChunkRepository) BulkInsertChunks(ctx context.Context, chunks []domain.RagChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	rows := make([][]interface{}, len(chunks))
	for i, chunk := range chunks {
		rows[i] = []interface{}{
			chunk.ID,
			chunk.VersionID,
			chunk.Ordinal,
			chunk.Content,
			chunk.Embedding,
			chunk.CreatedAt,
		}
	}

	_, err := r.getExecutor(ctx).CopyFrom(
		ctx,
		pgx.Identifier{"rag_chunks"},
		[]string{"id", "version_id", "ordinal", "content", "embedding", "created_at"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("failed to bulk insert chunks: %w", err)
	}

	return nil
}

// GetChunksByVersionID retrieves chunks for a version. Callers (chunk-diff
// computation in indexArticleUsecase.Upsert, retrieval context assembly in
// articleScopedStrategy) only ever read ID/Ordinal/Content — never
// Embedding — so the embedding column is deliberately excluded from both
// the SELECT and the scan. Scanning it into the non-nullable
// domain.RagChunk.Embedding would panic on any row with a NULL embedding
// (pgvector-go's Vector.DecodeBinary does not guard against a nil/empty
// buffer), which is the state of every pre-existing chunk after a
// dimension-widening migration re-nulls the column.
func (r *ragChunkRepository) GetChunksByVersionID(ctx context.Context, versionID uuid.UUID) ([]domain.RagChunk, error) {
	query := `
		SELECT id, version_id, ordinal, content, created_at
		FROM rag_chunks
		WHERE version_id = $1
		ORDER BY ordinal ASC
	`
	rows, err := r.getExecutor(ctx).Query(ctx, query, versionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var chunks []domain.RagChunk
	for rows.Next() {
		var c domain.RagChunk
		if err := rows.Scan(&c.ID, &c.VersionID, &c.Ordinal, &c.Content, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}
		chunks = append(chunks, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return chunks, nil
}

func (r *ragChunkRepository) InsertEvents(ctx context.Context, events []domain.RagChunkEvent) error {
	if len(events) == 0 {
		return nil
	}

	rows := make([][]interface{}, len(events))
	for i, event := range events {
		rows[i] = []interface{}{
			event.ID,
			event.VersionID,
			event.ChunkID,
			event.Ordinal,
			event.EventType,
			event.Metadata,
			event.CreatedAt,
		}
	}

	_, err := r.getExecutor(ctx).CopyFrom(
		ctx,
		pgx.Identifier{"rag_chunk_events"},
		[]string{"id", "version_id", "chunk_id", "ordinal", "event_type", "metadata", "created_at"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("failed to insert chunk events: %w", err)
	}

	return nil
}

// Overfetch bounds for the vector candidate pool. Candidates are scoped to
// the requested user_id and current document version inside the CTE, so the
// candidate pool contains only current versions belonging to the user before
// the outer query applies the final LIMIT $3.
const (
	searchOverfetchMultiplier = 4
	searchOverfetchCap        = 200
)

// Search performs a vector search across chunks belonging to userID (Augur use case).
//
// One round-trip: the candidate scan and the metadata enrichment are a single CTE query,
// filtered by rag_documents.user_id = userID (NULL-owner rows are invisible) within the
// candidate CTE before LIMIT $2 is applied to guarantee user recall.
func (r *ragChunkRepository) Search(ctx context.Context, queryVector []float32, limit int, userID uuid.UUID) ([]domain.SearchResult, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("ragChunkRepository.Search: user_id is required")
	}

	candidateLimit := limit * searchOverfetchMultiplier
	if candidateLimit > searchOverfetchCap {
		candidateLimit = searchOverfetchCap
	}

	// The candidates CTE filters by current document version and user_id before
	// applying candidateLimit ($2), ensuring other users' chunks cannot crowd out
	// this user's results.
	//
	// c.embedding is selected only inside the distance projection, never as its
	// own column: scanning it into the non-nullable domain.RagChunk.Embedding
	// panics on a NULL embedding, and no caller reads it.
	query := `
		WITH candidates AS (
			SELECT c.id, (c.embedding <=> $1) AS distance
			FROM rag_chunks c
			JOIN rag_document_versions v ON c.version_id = v.id
			JOIN rag_documents d ON v.document_id = d.id
			WHERE d.current_version_id = v.id
			  AND d.user_id = $4
			ORDER BY c.embedding <=> $1
			LIMIT $2
		)
		SELECT
			c.id, c.version_id, c.ordinal, c.content, c.created_at,
			d.article_id,
			v.version_number,
			v.title,
			v.url,
			cand.distance
		FROM candidates cand
		JOIN rag_chunks c ON c.id = cand.id
		JOIN rag_document_versions v ON c.version_id = v.id
		JOIN rag_documents d ON v.document_id = d.id
		WHERE d.current_version_id = v.id
		  AND d.user_id = $4
		ORDER BY cand.distance ASC
		LIMIT $3
	`

	rows, err := r.getExecutor(ctx).Query(ctx, query, pgvector.NewVector(queryVector), candidateLimit, limit, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to search chunks: %w", err)
	}
	defer rows.Close()

	var results []domain.SearchResult
	for rows.Next() {
		var c domain.RagChunk
		var articleID string
		var versionNumber int
		var title, url sql.NullString
		var distance float32
		if err := rows.Scan(&c.ID, &c.VersionID, &c.Ordinal, &c.Content, &c.CreatedAt, &articleID, &versionNumber, &title, &url, &distance); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}

		results = append(results, domain.SearchResult{
			Chunk:           c,
			Score:           1.0 - distance,
			ScoreKind:       domain.ScoreKindVector,
			ArticleID:       articleID,
			Title:           title.String,
			URL:             url.String,
			DocumentVersion: versionNumber,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return results, nil
}

// SearchWithinArticles performs a vector search within specific articles for userID (Morning Letter use case).
// Uses pre-filtering by article IDs and user_id before vector search.
// This is less efficient than Search() but necessary when filtering to a small subset of articles.
//
// c.embedding is selected only inside the "(c.embedding <=> $1) as distance"
// projection, never as its own column: scanning it into the non-nullable
// domain.RagChunk.Embedding (pgvector.Vector) would panic on any row whose
// embedding is NULL, exactly like the bug fixed in GetChunksByVersionID.
// This query has no "embedding IS NOT NULL" filter — a NULL embedding
// still produces a NULL distance and sorts last, it does not exclude the
// row — so a NULL-embedding chunk for a requested article reaches Scan.
// No caller reads SearchResult.Chunk.Embedding, so the column is dropped.
func (r *ragChunkRepository) SearchWithinArticles(ctx context.Context, queryVector []float32, articleIDs []string, limit int, userID uuid.UUID) ([]domain.SearchResult, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("ragChunkRepository.SearchWithinArticles: user_id is required")
	}
	if len(articleIDs) == 0 {
		return []domain.SearchResult{}, nil
	}

	query := `
		SELECT
			c.id, c.version_id, c.ordinal, c.content, c.created_at,
			d.article_id,
			v.version_number,
			v.title,
			v.url,
			(c.embedding <=> $1) as distance
		FROM rag_chunks c
		JOIN rag_document_versions v ON c.version_id = v.id
		JOIN rag_documents d ON v.document_id = d.id
		WHERE d.article_id = ANY($2)
		  AND d.current_version_id = v.id
		  AND d.user_id = $4
		ORDER BY distance ASC
		LIMIT $3
	`

	rows, err := r.getExecutor(ctx).Query(ctx, query, pgvector.NewVector(queryVector), articleIDs, limit, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to search within articles: %w", err)
	}
	defer rows.Close()

	var results []domain.SearchResult
	for rows.Next() {
		var c domain.RagChunk
		var articleID string
		var versionNumber int
		var title, url sql.NullString
		var distance float32
		if err := rows.Scan(&c.ID, &c.VersionID, &c.Ordinal, &c.Content, &c.CreatedAt, &articleID, &versionNumber, &title, &url, &distance); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}

		results = append(results, domain.SearchResult{
			Chunk:           c,
			Score:           1.0 - distance,
			ScoreKind:       domain.ScoreKindVector,
			ArticleID:       articleID,
			Title:           title.String,
			URL:             url.String,
			DocumentVersion: versionNumber,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return results, nil
}
