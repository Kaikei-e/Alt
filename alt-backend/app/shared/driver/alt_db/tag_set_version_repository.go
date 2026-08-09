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

// MarkTagSetVersionSuperseded points every tag set version of an article that
// is older than newVersionID at it. Returns the newest version it superseded,
// or nil when the new one arrived already beaten and superseded nothing.
//
// Same per-article advisory-lock transaction pattern as
// SummaryRepository.MarkSummaryVersionSuperseded — see its doc comment for
// why row-level UPDATE locking alone cannot serialize two concurrent calls
// for the same article, and for why the age comparison is what keeps the
// serialized calls from undoing each other.
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

	// Age of the incoming version, read under the lock. A caller naming a
	// version that does not exist is a wiring fault, and answering it with a
	// silent no-op would leave the article's tags frozen with nothing said.
	var newGeneratedAt time.Time
	err = tx.QueryRow(ctx,
		`SELECT generated_at FROM tag_set_versions WHERE tag_set_version_id = $1`,
		newVersionID).Scan(&newGeneratedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("MarkTagSetVersionSuperseded: tag set version %s does not exist", newVersionID)
		}
		return nil, fmt.Errorf("MarkTagSetVersionSuperseded read new version: %w", err)
	}

	// Age decides, with the id breaking ties so two versions stamped in the
	// same instant still order deterministically. The old predicate was
	// "everything that is not me", which let a version supersede one generated
	// after it: two regenerations racing on one article each superseded the
	// other and the article was left with no current tag set at all.
	//
	// A version can also arrive after a newer one has already won — the batch
	// job's rows are older than what the on-the-fly path produces while it
	// runs. It loses, and says so by pointing at the winner. Either way the
	// article comes out of this transaction with exactly one version that no
	// other supersedes.
	var winner uuid.UUID
	err = tx.QueryRow(ctx, `SELECT tag_set_version_id FROM tag_set_versions
		WHERE article_id = $1 AND superseded_by IS NULL
		  AND (generated_at, tag_set_version_id) > ($2, $3)
		ORDER BY generated_at DESC, tag_set_version_id DESC
		LIMIT 1`, articleID, newGeneratedAt, newVersionID).Scan(&winner)
	switch {
	case err == nil:
		if _, err := tx.Exec(ctx, `UPDATE tag_set_versions SET superseded_by = $1
			WHERE tag_set_version_id = $2 AND superseded_by IS NULL`, winner, newVersionID); err != nil {
			return nil, fmt.Errorf("MarkTagSetVersionSuperseded yield to newer: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("MarkTagSetVersionSuperseded commit: %w", err)
		}
		// Superseded nothing, so it has no predecessor to announce.
		return nil, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, fmt.Errorf("MarkTagSetVersionSuperseded select newer: %w", err)
	}

	const olderThanNew = `article_id = $1 AND superseded_by IS NULL
		AND (generated_at, tag_set_version_id) < ($2, $3)`

	var prev domain.TagSetVersion
	err = tx.QueryRow(ctx, `SELECT tag_set_version_id, article_id, user_id, generated_at,
		generator, input_hash, tags_json, superseded_by
		FROM tag_set_versions
		WHERE `+olderThanNew+`
		ORDER BY generated_at DESC, tag_set_version_id DESC
		LIMIT 1`, articleID, newGeneratedAt, newVersionID).Scan(
		&prev.TagSetVersionID, &prev.ArticleID, &prev.UserID, &prev.GeneratedAt,
		&prev.Generator, &prev.InputHash, &prev.TagsJSON, &prev.SupersededBy,
	)
	switch {
	case err == nil:
		if _, err := tx.Exec(ctx, `UPDATE tag_set_versions
			SET superseded_by = $4
			WHERE `+olderThanNew, articleID, newGeneratedAt, newVersionID, newVersionID); err != nil {
			return nil, fmt.Errorf("MarkTagSetVersionSuperseded update: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("MarkTagSetVersionSuperseded commit: %w", err)
		}
		return &prev, nil
	case errors.Is(err, pgx.ErrNoRows):
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("MarkTagSetVersionSuperseded commit: %w", err)
		}
		return nil, nil // First version of this article; nothing to supersede.
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
