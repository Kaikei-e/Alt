package alt_db

import (
	"alt/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

// CreateTagSetVersion inserts a new tag set version.
func (r *AltDBRepository) CreateTagSetVersion(ctx context.Context, tsv domain.TagSetVersion) error {
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

// GetTagSetVersionByID reads a specific tag set version by its ID.
func (r *AltDBRepository) GetTagSetVersionByID(ctx context.Context, tagSetVersionID uuid.UUID) (domain.TagSetVersion, error) {
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
