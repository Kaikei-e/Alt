package repository

import (
	"context"
	"database/sql"
	"fmt"
	"rag-orchestrator/internal/domain"
	"sort"

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

// Search performs a vector search across all chunks (Augur use case).
// Uses Two-Stage Search for HNSW index efficiency.
func (r *ragChunkRepository) Search(ctx context.Context, queryVector []float32, limit int) ([]domain.SearchResult, error) {
	// Two-Stage Search for HNSW Index Efficiency
	//
	// Stage 1: Pure vector search on rag_chunks (uses HNSW index efficiently)
	// Stage 2: Enrich with metadata via JOIN (filters to current version only)
	//
	// This approach ensures HNSW index is used in Stage 1, then filters/enriches
	// in Stage 2 with a smaller candidate set.

	// Fetch more candidates in Stage 1 to account for filtering in Stage 2
	// (some chunks may belong to non-current versions)
	candidateMultiplier := 3
	stage1Limit := limit * candidateMultiplier
	if stage1Limit > 500 {
		stage1Limit = 500 // Cap to prevent excessive memory usage
	}

	// Stage 1: Pure vector search (HNSW optimized)
	stage1Query := `
		SELECT c.id, (c.embedding <=> $1) as distance
		FROM rag_chunks c
		ORDER BY distance ASC
		LIMIT $2
	`
	stage1Rows, err := r.getExecutor(ctx).Query(ctx, stage1Query, pgvector.NewVector(queryVector), stage1Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search chunks (stage 1): %w", err)
	}

	// Collect chunk IDs and distances from Stage 1
	type chunkCandidate struct {
		id       uuid.UUID
		distance float32
	}
	candidates := make([]chunkCandidate, 0, stage1Limit)
	chunkIDs := make([]uuid.UUID, 0, stage1Limit)

	for stage1Rows.Next() {
		var id uuid.UUID
		var distance float32
		if err := stage1Rows.Scan(&id, &distance); err != nil {
			stage1Rows.Close()
			return nil, fmt.Errorf("failed to scan stage 1 result: %w", err)
		}
		candidates = append(candidates, chunkCandidate{id: id, distance: distance})
		chunkIDs = append(chunkIDs, id)
	}
	stage1Rows.Close()
	if err := stage1Rows.Err(); err != nil {
		return nil, fmt.Errorf("stage 1 rows error: %w", err)
	}

	if len(candidates) == 0 {
		return []domain.SearchResult{}, nil
	}

	// Build distance lookup map
	distanceMap := make(map[uuid.UUID]float32, len(candidates))
	for _, c := range candidates {
		distanceMap[c.id] = c.distance
	}

	// Stage 2: Enrich with metadata, filter by current version only
	stage2Query := `
		SELECT
			c.id, c.version_id, c.ordinal, c.content, c.embedding, c.created_at,
			d.article_id,
			v.version_number,
			v.title,
			v.url
		FROM rag_chunks c
		JOIN rag_document_versions v ON c.version_id = v.id
		JOIN rag_documents d ON v.document_id = d.id
		WHERE c.id = ANY($1)
		  AND d.current_version_id = v.id
	`

	stage2Rows, err := r.getExecutor(ctx).Query(ctx, stage2Query, chunkIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to enrich chunks (stage 2): %w", err)
	}
	defer stage2Rows.Close()

	var results []domain.SearchResult
	for stage2Rows.Next() {
		var c domain.RagChunk
		var articleID string
		var versionNumber int
		var title, url sql.NullString
		if err := stage2Rows.Scan(&c.ID, &c.VersionID, &c.Ordinal, &c.Content, &c.Embedding, &c.CreatedAt, &articleID, &versionNumber, &title, &url); err != nil {
			return nil, fmt.Errorf("failed to scan stage 2 result: %w", err)
		}

		distance := distanceMap[c.ID]
		results = append(results, domain.SearchResult{
			Chunk:           c,
			Score:           1.0 - distance,
			ArticleID:       articleID,
			Title:           title.String,
			URL:             url.String,
			DocumentVersion: versionNumber,
		})
	}
	if err := stage2Rows.Err(); err != nil {
		return nil, fmt.Errorf("stage 2 rows error: %w", err)
	}

	// Sort by distance (score descending) and limit
	// Results from Stage 2 are not ordered, so we need to sort
	sortByDistance(results)

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// SearchWithinArticles performs a vector search within specific articles (Morning Letter use case).
// Uses pre-filtering by article IDs before vector search.
// This is less efficient than Search() but necessary when filtering to a small subset of articles.
func (r *ragChunkRepository) SearchWithinArticles(ctx context.Context, queryVector []float32, articleIDs []string, limit int) ([]domain.SearchResult, error) {
	if len(articleIDs) == 0 {
		return []domain.SearchResult{}, nil
	}

	// Single-pass query with pre-filtering by article IDs
	// Note: HNSW index cannot be used efficiently with this approach,
	// but since we're filtering to a small subset of articles, performance is acceptable.
	query := `
		SELECT
			c.id, c.version_id, c.ordinal, c.content, c.embedding, c.created_at,
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
		ORDER BY distance ASC
		LIMIT $3
	`

	rows, err := r.getExecutor(ctx).Query(ctx, query, pgvector.NewVector(queryVector), articleIDs, limit)
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
		if err := rows.Scan(&c.ID, &c.VersionID, &c.Ordinal, &c.Content, &c.Embedding, &c.CreatedAt, &articleID, &versionNumber, &title, &url, &distance); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}

		results = append(results, domain.SearchResult{
			Chunk:           c,
			Score:           1.0 - distance,
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

// sortByDistance sorts search results by score in descending order (higher score = more similar)
func sortByDistance(results []domain.SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}
