package alt_db

import (
	"alt/domain"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// CreateSummaryVersion inserts a new summary version.
func (r *SummaryRepository) CreateSummaryVersion(ctx context.Context, sv domain.SummaryVersion) error {
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

// MarkSummaryVersionSuperseded points every summary version of an article that
// is older than newVersionID at it. Returns the newest version it superseded,
// or nil when the new one arrived already beaten and superseded nothing.
//
// The select-then-update pair runs inside a transaction guarded by a
// per-article pg_advisory_xact_lock. Without it, two summary versions
// created back-to-back for the same article can each run this method
// concurrently and read a "prev" the other is about to change: row-level
// UPDATE locking alone does not serialize them, because the candidate row
// sets are computed before either UPDATE runs. The advisory lock is keyed on
// article_id (not on the changing set of non-superseded rows) so the second
// call fully waits for the first transaction to commit before it reads.
//
// Serializing is necessary but not sufficient. The predicate below compares
// generation age, and the earlier "everything that is not me" form is what
// made two serialized calls undo each other: each superseded the other's row,
// and the article ended up with every version pointing at a successor and no
// current summary at all.
func (r *SummaryRepository) MarkSummaryVersionSuperseded(ctx context.Context, articleID uuid.UUID, newVersionID uuid.UUID) (*domain.SummaryVersion, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.MarkSummaryVersionSuperseded")
	defer span.End()

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("MarkSummaryVersionSuperseded begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1::text))", articleID); err != nil {
		return nil, fmt.Errorf("MarkSummaryVersionSuperseded advisory lock: %w", err)
	}

	// Age of the incoming version, read under the lock. A caller naming a
	// version that does not exist is a wiring fault, and answering it with a
	// silent no-op would leave the article's summary frozen with nothing said.
	var newGeneratedAt time.Time
	err = tx.QueryRow(ctx,
		`SELECT generated_at FROM summary_versions WHERE summary_version_id = $1`,
		newVersionID).Scan(&newGeneratedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("MarkSummaryVersionSuperseded: summary version %s does not exist", newVersionID)
		}
		return nil, fmt.Errorf("MarkSummaryVersionSuperseded read new version: %w", err)
	}

	// A version can arrive after a newer one has already won. It loses, and
	// says so by pointing at the winner, so the article leaves this
	// transaction with exactly one version that no other supersedes.
	var winner uuid.UUID
	err = tx.QueryRow(ctx, `SELECT summary_version_id FROM summary_versions
		WHERE article_id = $1 AND superseded_by IS NULL
		  AND (generated_at, summary_version_id) > ($2, $3)
		ORDER BY generated_at DESC, summary_version_id DESC
		LIMIT 1`, articleID, newGeneratedAt, newVersionID).Scan(&winner)
	switch {
	case err == nil:
		if _, err := tx.Exec(ctx, `UPDATE summary_versions SET superseded_by = $1
			WHERE summary_version_id = $2 AND superseded_by IS NULL`, winner, newVersionID); err != nil {
			return nil, fmt.Errorf("MarkSummaryVersionSuperseded yield to newer: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("MarkSummaryVersionSuperseded commit: %w", err)
		}
		// Superseded nothing, so it has no predecessor to announce.
		return nil, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, fmt.Errorf("MarkSummaryVersionSuperseded select newer: %w", err)
	}

	const olderThanNew = `article_id = $1 AND superseded_by IS NULL
		AND (generated_at, summary_version_id) < ($2, $3)`

	var prev domain.SummaryVersion
	err = tx.QueryRow(ctx, `SELECT summary_version_id, article_id, user_id, generated_at,
		model, prompt_version, input_hash, quality_score,
		summary_text, superseded_by
		FROM summary_versions
		WHERE `+olderThanNew+`
		ORDER BY generated_at DESC, summary_version_id DESC
		LIMIT 1`, articleID, newGeneratedAt, newVersionID).Scan(
		&prev.SummaryVersionID, &prev.ArticleID, &prev.UserID, &prev.GeneratedAt,
		&prev.Model, &prev.PromptVersion, &prev.InputHash, &prev.QualityScore,
		&prev.SummaryText, &prev.SupersededBy,
	)
	switch {
	case err == nil:
		if _, err := tx.Exec(ctx, `UPDATE summary_versions
			SET superseded_by = $4
			WHERE `+olderThanNew, articleID, newGeneratedAt, newVersionID, newVersionID); err != nil {
			return nil, fmt.Errorf("MarkSummaryVersionSuperseded update: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("MarkSummaryVersionSuperseded commit: %w", err)
		}
		return &prev, nil
	case errors.Is(err, pgx.ErrNoRows):
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("MarkSummaryVersionSuperseded commit: %w", err)
		}
		return nil, nil // First version of this article; nothing to supersede.
	default:
		return nil, fmt.Errorf("MarkSummaryVersionSuperseded select: %w", err)
	}
}

// GetSummaryVersionByID returns a specific summary version by its ID.
// This is reproject-safe: replaying an old event will always fetch the correct version.
func (r *SummaryRepository) GetSummaryVersionByID(ctx context.Context, summaryVersionID uuid.UUID) (domain.SummaryVersion, error) {
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
func (r *SummaryRepository) GetLatestSummaryVersion(ctx context.Context, articleID uuid.UUID) (domain.SummaryVersion, error) {
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
