package alt_db

import (
	"alt/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// CreateSummaryVersion inserts a new summary version.
func (r *AltDBRepository) CreateSummaryVersion(ctx context.Context, sv domain.SummaryVersion) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.CreateSummaryVersion")
	defer span.End()

	query := `INSERT INTO summary_versions
		(summary_version_id, article_id, user_id, generated_at,
		 model, prompt_version, input_hash, quality_score,
		 summary_text, superseded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := r.pool.Exec(ctx, query,
		sv.SummaryVersionID, sv.ArticleID, sv.UserID, sv.GeneratedAt,
		sv.Model, sv.PromptVersion, sv.InputHash, sv.QualityScore,
		sv.SummaryText, sv.SupersededBy,
	)
	if err != nil {
		return fmt.Errorf("CreateSummaryVersion: %w", err)
	}

	return nil
}

// GetLatestSummaryVersion returns the latest non-superseded summary version for an article.
func (r *AltDBRepository) GetLatestSummaryVersion(ctx context.Context, articleID uuid.UUID) (domain.SummaryVersion, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetLatestSummaryVersion")
	defer span.End()

	query := `SELECT summary_version_id, article_id, user_id, generated_at,
		model, prompt_version, input_hash, quality_score,
		summary_text, superseded_by
		FROM summary_versions
		WHERE article_id = $1 AND superseded_by IS NULL
		ORDER BY generated_at DESC
		LIMIT 1`

	var sv domain.SummaryVersion
	err := r.pool.QueryRow(ctx, query, articleID).Scan(
		&sv.SummaryVersionID, &sv.ArticleID, &sv.UserID, &sv.GeneratedAt,
		&sv.Model, &sv.PromptVersion, &sv.InputHash, &sv.QualityScore,
		&sv.SummaryText, &sv.SupersededBy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.SummaryVersion{}, fmt.Errorf("no summary version found for article %s", articleID)
		}
		return domain.SummaryVersion{}, fmt.Errorf("GetLatestSummaryVersion: %w", err)
	}

	return sv, nil
}
