package alt_db

import (
	"alt/domain"
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// GetActiveProjectionVersion returns the currently active projection version.
func (r *AltDBRepository) GetActiveProjectionVersion(ctx context.Context) (*domain.KnowledgeProjectionVersion, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetActiveProjectionVersion")
	defer span.End()

	query := `SELECT version, description, status, created_at, activated_at
		FROM knowledge_projection_versions WHERE status = 'active' ORDER BY version DESC LIMIT 1`

	var v domain.KnowledgeProjectionVersion
	err := r.pool.QueryRow(ctx, query).Scan(&v.Version, &v.Description, &v.Status, &v.CreatedAt, &v.ActivatedAt)
	if err != nil {
		return nil, fmt.Errorf("GetActiveProjectionVersion: %w", err)
	}
	return &v, nil
}

// ListProjectionVersions returns all projection versions.
func (r *AltDBRepository) ListProjectionVersions(ctx context.Context) ([]domain.KnowledgeProjectionVersion, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ListProjectionVersions")
	defer span.End()

	query := `SELECT version, description, status, created_at, activated_at
		FROM knowledge_projection_versions ORDER BY version DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ListProjectionVersions: %w", err)
	}
	defer rows.Close()

	var versions []domain.KnowledgeProjectionVersion
	for rows.Next() {
		var v domain.KnowledgeProjectionVersion
		if err := rows.Scan(&v.Version, &v.Description, &v.Status, &v.CreatedAt, &v.ActivatedAt); err != nil {
			return nil, fmt.Errorf("ListProjectionVersions scan: %w", err)
		}
		versions = append(versions, v)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(versions)))
	return versions, nil
}

// CreateProjectionVersion inserts a new projection version.
func (r *AltDBRepository) CreateProjectionVersion(ctx context.Context, v domain.KnowledgeProjectionVersion) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.CreateProjectionVersion")
	defer span.End()

	query := `INSERT INTO knowledge_projection_versions (version, description, status, created_at, activated_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, query, v.Version, v.Description, v.Status, v.CreatedAt, v.ActivatedAt)
	if err != nil {
		return fmt.Errorf("CreateProjectionVersion: %w", err)
	}
	return nil
}

// ActivateProjectionVersion activates a specific version and deactivates all others.
func (r *AltDBRepository) ActivateProjectionVersion(ctx context.Context, version int) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ActivateProjectionVersion")
	defer span.End()

	// Deactivate all other versions
	_, err := r.pool.Exec(ctx,
		`UPDATE knowledge_projection_versions SET status = 'inactive' WHERE version != $1 AND status = 'active'`,
		version)
	if err != nil {
		return fmt.Errorf("ActivateProjectionVersion deactivate: %w", err)
	}

	// Activate the target version
	now := time.Now()
	commandTag, err := r.pool.Exec(ctx,
		`UPDATE knowledge_projection_versions SET status = 'active', activated_at = $2 WHERE version = $1`,
		version, now)
	if err != nil {
		return fmt.Errorf("ActivateProjectionVersion activate: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("ActivateProjectionVersion: version %d not found in knowledge_projection_versions", version)
	}

	return nil
}
