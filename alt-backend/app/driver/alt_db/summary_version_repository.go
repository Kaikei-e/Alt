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

// MarkSummaryVersionSuperseded marks all non-superseded summary versions for an article as superseded
// by the new version. Returns the previous latest version before marking, or nil if none existed.
func (r *AltDBRepository) MarkSummaryVersionSuperseded(ctx context.Context, articleID uuid.UUID, newVersionID uuid.UUID) (*domain.SummaryVersion, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.MarkSummaryVersionSuperseded")
	defer span.End()

	// First, get the current latest version (before superseding)
	var prev domain.SummaryVersion
	selectQuery := `SELECT summary_version_id, article_id, user_id, generated_at,
		model, prompt_version, input_hash, quality_score,
		summary_text, superseded_by
		FROM summary_versions
		WHERE article_id = $1 AND superseded_by IS NULL AND summary_version_id != $2
		ORDER BY generated_at DESC
		LIMIT 1`

	err := r.pool.QueryRow(ctx, selectQuery, articleID, newVersionID).Scan(
		&prev.SummaryVersionID, &prev.ArticleID, &prev.UserID, &prev.GeneratedAt,
		&prev.Model, &prev.PromptVersion, &prev.InputHash, &prev.QualityScore,
		&prev.SummaryText, &prev.SupersededBy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No previous version
		}
		return nil, fmt.Errorf("MarkSummaryVersionSuperseded select: %w", err)
	}

	// Mark all non-superseded versions (except the new one) as superseded
	updateQuery := `UPDATE summary_versions
		SET superseded_by = $1
		WHERE article_id = $2 AND superseded_by IS NULL AND summary_version_id != $1`

	_, err = r.pool.Exec(ctx, updateQuery, newVersionID, articleID)
	if err != nil {
		return nil, fmt.Errorf("MarkSummaryVersionSuperseded update: %w", err)
	}

	return &prev, nil
}

// GetSummaryVersionByID returns a specific summary version by its ID.
// This is reproject-safe: replaying an old event will always fetch the correct version.
func (r *AltDBRepository) GetSummaryVersionByID(ctx context.Context, summaryVersionID uuid.UUID) (domain.SummaryVersion, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetSummaryVersionByID")
	defer span.End()

	query := `SELECT summary_version_id, article_id, user_id, generated_at,
		model, prompt_version, input_hash, quality_score,
		summary_text, superseded_by
		FROM summary_versions
		WHERE summary_version_id = $1`

	var sv domain.SummaryVersion
	err := r.pool.QueryRow(ctx, query, summaryVersionID).Scan(
		&sv.SummaryVersionID, &sv.ArticleID, &sv.UserID, &sv.GeneratedAt,
		&sv.Model, &sv.PromptVersion, &sv.InputHash, &sv.QualityScore,
		&sv.SummaryText, &sv.SupersededBy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.SummaryVersion{}, fmt.Errorf("no summary version found for id %s", summaryVersionID)
		}
		return domain.SummaryVersion{}, fmt.Errorf("GetSummaryVersionByID: %w", err)
	}

	return sv, nil
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
