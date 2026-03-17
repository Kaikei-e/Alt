package alt_db

import (
	"alt/domain"
	"context"
	"fmt"

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
