package sovereign_db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ProjectionVersion represents a projection version record.
type ProjectionVersion struct {
	Version     int
	Description string
	Status      string
	CreatedAt   time.Time
	ActivatedAt *time.Time
}

// GetActiveProjectionVersion returns the currently active projection version.
func (r *Repository) GetActiveProjectionVersion(ctx context.Context) (*ProjectionVersion, error) {
	query := `SELECT version, description, status, created_at, activated_at
		FROM knowledge_projection_versions WHERE status = 'active'
		ORDER BY version DESC LIMIT 1`

	v, err := scanProjectionVersion(r.pool.QueryRow(ctx, query))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetActiveProjectionVersion: %w", err)
	}
	return &v, nil
}

// ListProjectionVersions returns all projection versions.
func (r *Repository) ListProjectionVersions(ctx context.Context) ([]ProjectionVersion, error) {
	query := `SELECT version, description, status, created_at, activated_at
		FROM knowledge_projection_versions ORDER BY version DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ListProjectionVersions: %w", err)
	}
	defer rows.Close()

	return scanProjectionVersions(rows)
}

// CreateProjectionVersion inserts a new projection version.
func (r *Repository) CreateProjectionVersion(ctx context.Context, v ProjectionVersion) error {
	query := `INSERT INTO knowledge_projection_versions (version, description, status, created_at, activated_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, query, buildCreateProjectionVersionArgs(v)...)
	if err != nil {
		return fmt.Errorf("CreateProjectionVersion: %w", err)
	}
	return nil
}

// ActivateProjectionVersion sets a version as active and deactivates all
// others, atomically. A mid-failure (or an invalid version argument) must
// never leave zero active versions — that would silently regress every
// reader's COALESCE(...,1) fallback to projection v1. So this: (1) checks
// the target version exists BEFORE touching anything, and (2) performs the
// deactivate+activate pair inside a single transaction so a crash between
// the two statements can never be observed as "no active version".
func (r *Repository) ActivateProjectionVersion(ctx context.Context, version int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ActivateProjectionVersion begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit has succeeded

	existsQuery := `SELECT 1 FROM knowledge_projection_versions WHERE version = $1`
	var exists int
	if err := tx.QueryRow(ctx, existsQuery, version).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ActivateProjectionVersion: version %d not found", version)
		}
		return fmt.Errorf("ActivateProjectionVersion exists check: %w", err)
	}

	deactivateQuery := `UPDATE knowledge_projection_versions SET status = 'inactive', activated_at = NULL
		WHERE status = 'active' AND version != $1`
	if _, err := tx.Exec(ctx, deactivateQuery, version); err != nil {
		return fmt.Errorf("ActivateProjectionVersion deactivate: %w", err)
	}

	activateQuery := `UPDATE knowledge_projection_versions SET status = 'active', activated_at = now() WHERE version = $1`
	commandTag, err := tx.Exec(ctx, activateQuery, version)
	if err != nil {
		return fmt.Errorf("ActivateProjectionVersion: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("ActivateProjectionVersion: version %d not found", version)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ActivateProjectionVersion commit: %w", err)
	}
	return nil
}

func scanProjectionVersion(row rowScanner) (ProjectionVersion, error) {
	var v ProjectionVersion
	err := row.Scan(&v.Version, &v.Description, &v.Status, &v.CreatedAt, &v.ActivatedAt)
	return v, err
}

func scanProjectionVersions(rows pgx.Rows) ([]ProjectionVersion, error) {
	var versions []ProjectionVersion
	for rows.Next() {
		v, err := scanProjectionVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("ListProjectionVersions scan: %w", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListProjectionVersions rows: %w", err)
	}
	return versions, nil
}

func buildCreateProjectionVersionArgs(v ProjectionVersion) []any {
	return []any{v.Version, v.Description, v.Status, v.CreatedAt, v.ActivatedAt}
}
