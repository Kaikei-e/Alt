package alt_db

import (
	"alt/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// CreateTagSetVersion inserts a new tag set version.
func (r *TagRepository) CreateTagSetVersion(ctx context.Context, tsv domain.TagSetVersion) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.CreateTagSetVersion")
	defer span.End()

	query := `INSERT INTO tag_set_versions
		(tag_set_version_id, article_id, user_id, generated_at,
		 generator, input_hash, tags_json, superseded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.pool.Exec(ctx, query,
		tsv.TagSetVersionID, tsv.ArticleID, tsv.UserID, tsv.GeneratedAt,
		tsv.Generator, tsv.InputHash, tsv.TagsJSON, tsv.SupersededBy,
	)
	if err != nil {
		return fmt.Errorf("CreateTagSetVersion: %w", err)
	}

	return nil
}

// MarkTagSetVersionSuperseded marks all non-superseded tag set versions for an article as superseded
// by the new version. Returns the previous latest version before marking, or nil if none existed.
//
// Same per-article advisory-lock transaction pattern as
// SummaryRepository.MarkSummaryVersionSuperseded — see its doc comment for
// why row-level UPDATE locking alone cannot serialize two concurrent calls
// for the same article.
func (r *TagRepository) MarkTagSetVersionSuperseded(ctx context.Context, articleID uuid.UUID, newVersionID uuid.UUID) (*domain.TagSetVersion, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.MarkTagSetVersionSuperseded")
	defer span.End()

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("MarkTagSetVersionSuperseded begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1::text))", articleID); err != nil {
		return nil, fmt.Errorf("MarkTagSetVersionSuperseded advisory lock: %w", err)
	}

	// First, get the current latest version (before superseding)
	var prev domain.TagSetVersion
	selectQuery := `SELECT tag_set_version_id, article_id, user_id, generated_at,
		generator, input_hash, tags_json, superseded_by
		FROM tag_set_versions
		WHERE article_id = $1 AND superseded_by IS NULL AND tag_set_version_id != $2
		ORDER BY generated_at DESC
		LIMIT 1`

	err = tx.QueryRow(ctx, selectQuery, articleID, newVersionID).Scan(
		&prev.TagSetVersionID, &prev.ArticleID, &prev.UserID, &prev.GeneratedAt,
		&prev.Generator, &prev.InputHash, &prev.TagsJSON, &prev.SupersededBy,
	)
	switch err {
	case nil:
		// Mark all non-superseded versions (except the new one) as superseded
		updateQuery := `UPDATE tag_set_versions
			SET superseded_by = $1
			WHERE article_id = $2 AND superseded_by IS NULL AND tag_set_version_id != $1`

		if _, err := tx.Exec(ctx, updateQuery, newVersionID, articleID); err != nil {
			return nil, fmt.Errorf("MarkTagSetVersionSuperseded update: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("MarkTagSetVersionSuperseded commit: %w", err)
		}
		return &prev, nil
	case pgx.ErrNoRows:
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("MarkTagSetVersionSuperseded commit: %w", err)
		}
		return nil, nil // No previous version
	default:
		return nil, fmt.Errorf("MarkTagSetVersionSuperseded select: %w", err)
	}
}

// GetTagSetVersionByID reads a specific tag set version by its ID.
func (r *TagRepository) GetTagSetVersionByID(ctx context.Context, tagSetVersionID uuid.UUID) (domain.TagSetVersion, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetTagSetVersionByID")
	defer span.End()

	query := `SELECT tag_set_version_id, article_id, user_id, generated_at,
	                 generator, input_hash, tags_json, superseded_by
	          FROM tag_set_versions
	          WHERE tag_set_version_id = $1`

	var tsv domain.TagSetVersion
	err := r.pool.QueryRow(ctx, query, tagSetVersionID).Scan(
		&tsv.TagSetVersionID, &tsv.ArticleID, &tsv.UserID, &tsv.GeneratedAt,
		&tsv.Generator, &tsv.InputHash, &tsv.TagsJSON, &tsv.SupersededBy,
	)
	if err != nil {
		return domain.TagSetVersion{}, fmt.Errorf("GetTagSetVersionByID: %w", err)
	}

	return tsv, nil
}
