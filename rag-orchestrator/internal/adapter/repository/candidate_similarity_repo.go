package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/port"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type candidateSimilarityRepository struct {
	pool *pgxpool.Pool
}

// NewCandidateSimilarityRepository creates a repository for candidate-set
// similarity queries and current-version article metadata lookups.
func NewCandidateSimilarityRepository(pool *pgxpool.Pool) *candidateSimilarityRepository {
	return &candidateSimilarityRepository{pool: pool}
}

var (
	_ port.CandidateSimilarityRepository = (*candidateSimilarityRepository)(nil)
	_ port.ArticleMetadataReader         = (*candidateSimilarityRepository)(nil)
)

func (r *candidateSimilarityRepository) getExecutor(ctx context.Context) dbExecutor {
	tx := ExtractTx(ctx)
	if tx != nil {
		return tx
	}
	return r.pool
}

// ComputeCandidateSimilarities computes cosine similarities between seedArticleID
// and candidateArticleIDs in a single SQL query scoped to d.current_version_id
// and d.user_id = userID.
func (r *candidateSimilarityRepository) ComputeCandidateSimilarities(
	ctx context.Context,
	seedArticleID string,
	candidateArticleIDs []string,
	rep domain.SimilarityRepresentation,
	userID uuid.UUID,
) (*domain.CandidateSimilarityResult, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("candidateSimilarityRepository: user_id is required")
	}
	if strings.TrimSpace(seedArticleID) == "" {
		return nil, fmt.Errorf("candidateSimilarityRepository: seed_article_id is required")
	}
	if len(candidateArticleIDs) == 0 {
		return &domain.CandidateSimilarityResult{
			Results: []domain.CandidateSimilarityItem{},
		}, nil
	}

	query, err := buildCandidateSimilarityQuery(rep)
	if err != nil {
		return nil, err
	}

	rows, err := r.getExecutor(ctx).Query(ctx, query, seedArticleID, candidateArticleIDs, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to compute candidate similarities (%s): %w", rep, err)
	}
	defer rows.Close()

	var (
		results           []domain.CandidateSimilarityItem
		seedIndexed       bool
		indexedCandidates int
		nanDropped        int
	)
	for rows.Next() {
		var (
			rowSeedIndexed bool
			rowIndexedCand int
			articleID      sql.NullString
			similarity     sql.NullFloat64
		)
		if err := rows.Scan(&rowSeedIndexed, &rowIndexedCand, &articleID, &similarity); err != nil {
			return nil, fmt.Errorf("failed to scan candidate similarity row: %w", err)
		}
		seedIndexed = rowSeedIndexed
		indexedCandidates = rowIndexedCand

		if !articleID.Valid || !similarity.Valid {
			continue
		}
		if math.IsNaN(similarity.Float64) || math.IsInf(similarity.Float64, 0) {
			nanDropped++
			continue
		}
		results = append(results, domain.CandidateSimilarityItem{
			ArticleID:  articleID.String,
			Similarity: similarity.Float64,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("candidate similarity rows error: %w", err)
	}

	if results == nil {
		results = []domain.CandidateSimilarityItem{}
	}

	validIndexed := indexedCandidates - nanDropped
	if validIndexed < 0 {
		validIndexed = 0
	}
	missing := len(candidateArticleIDs) - validIndexed
	if missing < 0 {
		missing = 0
	}

	return &domain.CandidateSimilarityResult{
		Results:                    results,
		SeedIndexed:                seedIndexed,
		CandidatesWithoutEmbedding: missing,
	}, nil
}

// buildCandidateSimilarityQuery returns the single-statement SQL for rep.
// An anchor row (SELECT 1) LEFT JOINs the scored candidates so a single
// round-trip distinguishes "seed has no embeddings" from "no candidate has
// embeddings" while also reporting how many candidates have usable embeddings.
func buildCandidateSimilarityQuery(rep domain.SimilarityRepresentation) (string, error) {
	switch rep {
	case domain.SimilarityRepFirstChunk:
		return `
			WITH seed_vec AS (
				SELECT c.embedding
				FROM rag_documents d
				JOIN rag_chunks c ON d.current_version_id = c.version_id
				WHERE d.article_id = $1
				  AND d.user_id = $3
				  AND c.embedding IS NOT NULL
				ORDER BY c.ordinal ASC
				LIMIT 1
			),
			cand_vecs AS MATERIALIZED (
				SELECT d.article_id, fc.embedding
				FROM rag_documents d
				CROSS JOIN LATERAL (
					SELECT c.embedding
					FROM rag_chunks c
					WHERE d.current_version_id = c.version_id
					  AND c.embedding IS NOT NULL
					ORDER BY c.ordinal ASC
					LIMIT 1
				) fc
				WHERE d.article_id = ANY($2)
				  AND d.article_id <> $1
				  AND d.user_id = $3
			),
			scored AS (
				SELECT
					cv.article_id,
					(1.0 - (sv.embedding <=> cv.embedding))::double precision AS similarity
				FROM seed_vec sv
				CROSS JOIN cand_vecs cv
			)
			SELECT
				(SELECT EXISTS (SELECT 1 FROM seed_vec)) AS seed_indexed,
				(SELECT COUNT(*)::int FROM cand_vecs) AS indexed_candidates,
				s.article_id,
				s.similarity
			FROM (SELECT 1) AS anchor
			LEFT JOIN scored s ON TRUE
			ORDER BY s.similarity DESC NULLS LAST, s.article_id ASC
		`, nil

	case domain.SimilarityRepChunkMean:
		return `
			WITH seed_vec AS (
				SELECT AVG(c.embedding) AS embedding
				FROM rag_documents d
				JOIN rag_chunks c ON d.current_version_id = c.version_id
				WHERE d.article_id = $1
				  AND d.user_id = $3
				  AND c.embedding IS NOT NULL
				HAVING COUNT(c.embedding) > 0
			),
			cand_vecs AS MATERIALIZED (
				SELECT d.article_id, AVG(c.embedding) AS embedding
				FROM rag_documents d
				JOIN rag_chunks c ON d.current_version_id = c.version_id
				WHERE d.article_id = ANY($2)
				  AND d.article_id <> $1
				  AND d.user_id = $3
				  AND c.embedding IS NOT NULL
				GROUP BY d.article_id
			),
			scored AS (
				SELECT
					cv.article_id,
					(1.0 - (sv.embedding <=> cv.embedding))::double precision AS similarity
				FROM seed_vec sv
				CROSS JOIN cand_vecs cv
			)
			SELECT
				(SELECT EXISTS (SELECT 1 FROM seed_vec)) AS seed_indexed,
				(SELECT COUNT(*)::int FROM cand_vecs) AS indexed_candidates,
				s.article_id,
				s.similarity
			FROM (SELECT 1) AS anchor
			LEFT JOIN scored s ON TRUE
			ORDER BY s.similarity DESC NULLS LAST, s.article_id ASC
		`, nil

	case domain.SimilarityRepNearestChunkPair:
		return `
			WITH seed_chunks AS MATERIALIZED (
				SELECT c.embedding
				FROM rag_documents d
				JOIN rag_chunks c ON d.current_version_id = c.version_id
				WHERE d.article_id = $1
				  AND d.user_id = $3
				  AND c.embedding IS NOT NULL
			),
			cand_chunks AS MATERIALIZED (
				SELECT d.article_id, c.embedding
				FROM rag_documents d
				JOIN rag_chunks c ON d.current_version_id = c.version_id
				WHERE d.article_id = ANY($2)
				  AND d.article_id <> $1
				  AND d.user_id = $3
				  AND c.embedding IS NOT NULL
			),
			scored AS (
				SELECT
					cc.article_id,
					MAX(1.0 - (sc.embedding <=> cc.embedding))::double precision AS similarity
				FROM seed_chunks sc
				CROSS JOIN cand_chunks cc
				GROUP BY cc.article_id
			)
			SELECT
				(SELECT EXISTS (SELECT 1 FROM seed_chunks)) AS seed_indexed,
				(SELECT COUNT(DISTINCT cc.article_id)::int FROM cand_chunks cc) AS indexed_candidates,
				s.article_id,
				s.similarity
			FROM (SELECT 1) AS anchor
			LEFT JOIN scored s ON TRUE
			ORDER BY s.similarity DESC NULLS LAST, s.article_id ASC
		`, nil

	default:
		return "", fmt.Errorf("candidateSimilarityRepository: unsupported representation %q", rep)
	}
}

// GetArticleMetadataByIDs reads current-version titles and URLs for articleIDs
// owned by userID.
func (r *candidateSimilarityRepository) GetArticleMetadataByIDs(
	ctx context.Context,
	articleIDs []string,
	userID uuid.UUID,
) (map[string]domain.RagDocumentMetadata, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("candidateSimilarityRepository.GetArticleMetadataByIDs: user_id is required")
	}
	if len(articleIDs) == 0 {
		return map[string]domain.RagDocumentMetadata{}, nil
	}

	const query = `
		SELECT d.article_id, v.title, v.url
		FROM rag_documents d
		JOIN rag_document_versions v ON d.current_version_id = v.id
		WHERE d.article_id = ANY($1)
		  AND d.user_id = $2
	`
	rows, err := r.getExecutor(ctx).Query(ctx, query, articleIDs, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query article metadata: %w", err)
	}
	defer rows.Close()

	out := make(map[string]domain.RagDocumentMetadata, len(articleIDs))
	for rows.Next() {
		var (
			articleID string
			title     sql.NullString
			url       sql.NullString
		)
		if err := rows.Scan(&articleID, &title, &url); err != nil {
			return nil, fmt.Errorf("failed to scan article metadata: %w", err)
		}
		out[articleID] = domain.RagDocumentMetadata{
			ArticleID: articleID,
			Title:     title.String,
			URL:       url.String,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("article metadata rows error: %w", err)
	}
	return out, nil
}
