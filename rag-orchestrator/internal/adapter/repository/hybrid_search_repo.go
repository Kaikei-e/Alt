package repository

import (
	"context"
	"database/sql"
	"fmt"

	"rag-orchestrator/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// hybridSearchRepository performs in-database hybrid search (dense + sparse) with RRF.
type hybridSearchRepository struct {
	pool *pgxpool.Pool
	rrfK int // RRF constant k, typically 60
}

// NewHybridSearchRepository creates a new HybridSearcher.
// rrfK controls the RRF weighting (default 60).
func NewHybridSearchRepository(pool *pgxpool.Pool, rrfK int) domain.HybridSearcher {
	if rrfK <= 0 {
		rrfK = 60
	}
	return &hybridSearchRepository{pool: pool, rrfK: rrfK}
}

// HybridSearch performs a combined vector + full-text search with RRF fusion.
// Uses CTE-based approach:
// 1. vector_matches: HNSW cosine similarity search
// 2. text_matches: tsvector full-text search with ts_rank_cd
// 3. RRF fusion: 1/(rank + k) summed across both search methods
// 4. Metadata enrichment via JOIN
func (r *hybridSearchRepository) HybridSearch(ctx context.Context, queryVector []float32, queryText string, limit int) ([]domain.SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	// Candidate pool per method: 2x final limit for sufficient RRF coverage
	candidateLimit := limit * 2
	if candidateLimit > 100 {
		candidateLimit = 100
	}

	query := fmt.Sprintf(`
		WITH vector_matches AS (
			SELECT id, rank() OVER (ORDER BY embedding <=> $1) AS rank
			FROM rag_chunks
			ORDER BY embedding <=> $1
			LIMIT $3
		),
		text_matches AS (
			SELECT id, rank() OVER (ORDER BY ts_rank_cd(tsv, plainto_tsquery('english', $2)) DESC) AS rank
			FROM rag_chunks
			WHERE tsv @@ plainto_tsquery('english', $2)
			ORDER BY rank
			LIMIT $3
		),
		rrf AS (
			SELECT id, SUM(1.0 / (rank + %d)) AS score
			FROM (
				SELECT id, rank FROM vector_matches
				UNION ALL
				SELECT id, rank FROM text_matches
			) combined
			GROUP BY id
			ORDER BY score DESC
			LIMIT $4
		)
		SELECT
			r.score,
			c.id, c.version_id, c.ordinal, c.content, c.created_at,
			d.article_id,
			v.version_number,
			v.title,
			v.url
		FROM rrf r
		JOIN rag_chunks c ON r.id = c.id
		JOIN rag_document_versions v ON c.version_id = v.id
		JOIN rag_documents d ON v.document_id = d.id
		WHERE d.current_version_id = v.id
		ORDER BY r.score DESC
	`, r.rrfK)

	rows, err := r.pool.Query(ctx, query,
		pgvector.NewVector(queryVector),
		queryText,
		candidateLimit,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("hybrid search failed: %w", err)
	}
	defer rows.Close()

	var results []domain.SearchResult
	for rows.Next() {
		var score float32
		var chunk domain.RagChunk
		var articleID string
		var versionNumber int
		var title, url sql.NullString

		if err := rows.Scan(
			&score,
			&chunk.ID, &chunk.VersionID, &chunk.Ordinal, &chunk.Content, &chunk.CreatedAt,
			&articleID,
			&versionNumber,
			&title,
			&url,
		); err != nil {
			return nil, fmt.Errorf("failed to scan hybrid search result: %w", err)
		}

		results = append(results, domain.SearchResult{
			Chunk:           chunk,
			Score:           score,
			ArticleID:       articleID,
			Title:           title.String,
			URL:             url.String,
			DocumentVersion: versionNumber,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("hybrid search rows error: %w", err)
	}

	return results, nil
}
