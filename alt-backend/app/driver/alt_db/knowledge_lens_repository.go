package alt_db

import (
	"alt/domain"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (r *AltDBRepository) CreateLens(ctx context.Context, lens domain.KnowledgeLens) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.CreateLens")
	defer span.End()

	query := `INSERT INTO knowledge_lenses (lens_id, user_id, tenant_id, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, query,
		lens.LensID, lens.UserID, lens.TenantID, lens.Name, lens.Description, lens.CreatedAt, lens.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("CreateLens: %w", err)
	}
	return nil
}

func (r *AltDBRepository) CreateLensVersion(ctx context.Context, version domain.KnowledgeLensVersion) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.CreateLensVersion")
	defer span.End()

	tagIDsJSON, _ := json.Marshal(version.TagIDs)
	var timeWindowJSON []byte
	if version.TimeWindow != "" {
		timeWindowJSON, _ = json.Marshal(version.TimeWindow)
	}

	query := `INSERT INTO knowledge_lens_versions
		(lens_version_id, lens_id, created_at, query_text, tag_ids_json, time_window_json, include_recap, include_pulse, sort_mode)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.pool.Exec(ctx, query,
		version.LensVersionID, version.LensID, version.CreatedAt,
		version.QueryText, tagIDsJSON, timeWindowJSON,
		version.IncludeRecap, version.IncludePulse, version.SortMode,
	)
	if err != nil {
		return fmt.Errorf("CreateLensVersion: %w", err)
	}
	return nil
}

func (r *AltDBRepository) ListLenses(ctx context.Context, userID uuid.UUID) ([]domain.KnowledgeLens, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ListLenses")
	defer span.End()

	query := `SELECT lens_id, user_id, tenant_id, name, description, created_at, updated_at
		FROM knowledge_lenses
		WHERE user_id = $1 AND archived_at IS NULL
		ORDER BY updated_at DESC`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("ListLenses: %w", err)
	}
	defer rows.Close()

	var lenses []domain.KnowledgeLens
	for rows.Next() {
		var l domain.KnowledgeLens
		if err := rows.Scan(&l.LensID, &l.UserID, &l.TenantID, &l.Name, &l.Description, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("ListLenses scan: %w", err)
		}
		lenses = append(lenses, l)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(lenses)))
	return lenses, nil
}

func (r *AltDBRepository) GetLens(ctx context.Context, lensID uuid.UUID) (*domain.KnowledgeLens, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetLens")
	defer span.End()

	query := `SELECT lens_id, user_id, tenant_id, name, description, created_at, updated_at, archived_at
		FROM knowledge_lenses WHERE lens_id = $1`

	var l domain.KnowledgeLens
	err := r.pool.QueryRow(ctx, query, lensID).Scan(
		&l.LensID, &l.UserID, &l.TenantID, &l.Name, &l.Description, &l.CreatedAt, &l.UpdatedAt, &l.ArchivedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetLens: %w", err)
	}
	return &l, nil
}

func (r *AltDBRepository) GetCurrentLensVersion(ctx context.Context, lensID uuid.UUID) (*domain.KnowledgeLensVersion, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetCurrentLensVersion")
	defer span.End()

	query := `SELECT lens_version_id, lens_id, created_at, query_text, tag_ids_json,
		time_window_json, include_recap, include_pulse, sort_mode, superseded_by
		FROM knowledge_lens_versions
		WHERE lens_id = $1 AND superseded_by IS NULL
		ORDER BY created_at DESC LIMIT 1`

	var v domain.KnowledgeLensVersion
	var tagIDsJSON, timeWindowJSON []byte
	err := r.pool.QueryRow(ctx, query, lensID).Scan(
		&v.LensVersionID, &v.LensID, &v.CreatedAt, &v.QueryText,
		&tagIDsJSON, &timeWindowJSON, &v.IncludeRecap, &v.IncludePulse, &v.SortMode, &v.SupersededBy,
	)
	if err != nil {
		return nil, fmt.Errorf("GetCurrentLensVersion: %w", err)
	}
	_ = json.Unmarshal(tagIDsJSON, &v.TagIDs)
	if timeWindowJSON != nil {
		_ = json.Unmarshal(timeWindowJSON, &v.TimeWindow)
	}
	return &v, nil
}

func (r *AltDBRepository) SelectCurrentLens(ctx context.Context, current domain.KnowledgeCurrentLens) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.SelectCurrentLens")
	defer span.End()

	query := `INSERT INTO knowledge_current_lens (user_id, lens_id, lens_version_id, selected_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
		  lens_id = EXCLUDED.lens_id,
		  lens_version_id = EXCLUDED.lens_version_id,
		  selected_at = EXCLUDED.selected_at`
	_, err := r.pool.Exec(ctx, query, current.UserID, current.LensID, current.LensVersionID, current.SelectedAt)
	if err != nil {
		return fmt.Errorf("SelectCurrentLens: %w", err)
	}
	return nil
}

func (r *AltDBRepository) ArchiveLens(ctx context.Context, lensID uuid.UUID) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ArchiveLens")
	defer span.End()

	query := `UPDATE knowledge_lenses SET archived_at = now(), updated_at = now() WHERE lens_id = $1`
	_, err := r.pool.Exec(ctx, query, lensID)
	if err != nil {
		return fmt.Errorf("ArchiveLens: %w", err)
	}
	return nil
}
