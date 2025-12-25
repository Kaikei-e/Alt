package repository

import (
	"context"
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

func (r *ragChunkRepository) GetChunksByVersionID(ctx context.Context, versionID uuid.UUID) ([]domain.RagChunk, error) {
	query := `
		SELECT id, version_id, ordinal, content, embedding, created_at
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
		if err := rows.Scan(&c.ID, &c.VersionID, &c.Ordinal, &c.Content, &c.Embedding, &c.CreatedAt); err != nil {
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

func (r *ragChunkRepository) Search(ctx context.Context, queryVector []float32, candidateArticleIDs []string, limit int) ([]domain.SearchResult, error) {
	// Construct query
	// We want chunks from the *current version* of documents.
	baseQuery := `
		SELECT
			c.id, c.version_id, c.ordinal, c.content, c.embedding, c.created_at,
			(c.embedding <=> $1) as distance,
			d.article_id,
			v.version_number
		FROM rag_chunks c
		JOIN rag_document_versions v ON c.version_id = v.id
		JOIN rag_documents d ON v.document_id = d.id
		WHERE d.current_version_id = v.id
	`
	args := []interface{}{pgvector.NewVector(queryVector)}
	argIdx := 2

	if len(candidateArticleIDs) > 0 {
		baseQuery += fmt.Sprintf(" AND d.article_id = ANY($%d)", argIdx)
		args = append(args, candidateArticleIDs)
		argIdx++
	}

	baseQuery += fmt.Sprintf(" ORDER BY distance ASC LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := r.getExecutor(ctx).Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search chunks: %w", err)
	}
	defer rows.Close()

	var results []domain.SearchResult
	for rows.Next() {
		var c domain.RagChunk
		var distance float32
		var articleID string
		var versionNumber int
		if err := rows.Scan(&c.ID, &c.VersionID, &c.Ordinal, &c.Content, &c.Embedding, &c.CreatedAt, &distance, &articleID, &versionNumber); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		// Convert distance to similarity score (Approximate)
		results = append(results, domain.SearchResult{
			Chunk:           c,
			Score:           1.0 - distance,
			ArticleID:       articleID,
			DocumentVersion: versionNumber,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return results, nil
}
