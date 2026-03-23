package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// KnowledgeLens represents a saved viewpoint.
type KnowledgeLens struct {
	LensID         uuid.UUID
	UserID         uuid.UUID
	TenantID       uuid.UUID
	Name           string
	Description    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ArchivedAt     *time.Time
	CurrentVersion *KnowledgeLensVersion
}

// KnowledgeLensVersion represents a versioned filter config.
type KnowledgeLensVersion struct {
	LensVersionID uuid.UUID
	LensID        uuid.UUID
	CreatedAt     time.Time
	QueryText     string
	TagIDs        []string
	SourceIDs     []string
	TimeWindow    string
	IncludeRecap  bool
	IncludePulse  bool
	SortMode      string
	SupersededBy  *uuid.UUID
}

// KnowledgeCurrentLens represents the user's current lens selection.
type KnowledgeCurrentLens struct {
	UserID        uuid.UUID
	LensID        uuid.UUID
	LensVersionID uuid.UUID
	SelectedAt    time.Time
}

// ListLenses returns lenses for a user with their current versions.
func (r *Repository) ListLenses(ctx context.Context, userID uuid.UUID) ([]KnowledgeLens, error) {
	query := `SELECT l.lens_id, l.user_id, l.tenant_id, l.name, l.description,
		l.created_at, l.updated_at, l.archived_at,
		lv.lens_version_id, lv.lens_id, lv.created_at, lv.query_text,
		lv.tag_ids_json, lv.source_ids_json, lv.time_window_json,
		lv.include_recap, lv.include_pulse, lv.sort_mode, lv.superseded_by
		FROM knowledge_lenses l
		LEFT JOIN LATERAL (
			SELECT * FROM knowledge_lens_versions
			WHERE lens_id = l.lens_id AND superseded_by IS NULL
			ORDER BY created_at DESC LIMIT 1
		) lv ON true
		WHERE l.user_id = $1 AND l.archived_at IS NULL
		ORDER BY l.updated_at DESC`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("ListLenses: %w", err)
	}
	defer rows.Close()

	var lenses []KnowledgeLens
	for rows.Next() {
		var l KnowledgeLens
		var versionID *uuid.UUID
		var vLensID *uuid.UUID
		var vCreatedAt *time.Time
		var vQueryText *string
		var vTagIDsJSON, vSourceIDsJSON []byte
		var vTimeWindow *string
		var vIncludeRecap, vIncludePulse *bool
		var vSortMode *string
		var vSupersededBy *uuid.UUID

		if err := rows.Scan(
			&l.LensID, &l.UserID, &l.TenantID, &l.Name, &l.Description,
			&l.CreatedAt, &l.UpdatedAt, &l.ArchivedAt,
			&versionID, &vLensID, &vCreatedAt, &vQueryText,
			&vTagIDsJSON, &vSourceIDsJSON, &vTimeWindow,
			&vIncludeRecap, &vIncludePulse, &vSortMode, &vSupersededBy,
		); err != nil {
			return nil, fmt.Errorf("ListLenses scan: %w", err)
		}

		if versionID != nil {
			v := &KnowledgeLensVersion{
				LensVersionID: *versionID,
				LensID:        *vLensID,
				SupersededBy:  vSupersededBy,
			}
			if vCreatedAt != nil {
				v.CreatedAt = *vCreatedAt
			}
			if vQueryText != nil {
				v.QueryText = *vQueryText
			}
			if vTimeWindow != nil {
				v.TimeWindow = *vTimeWindow
			}
			if vIncludeRecap != nil {
				v.IncludeRecap = *vIncludeRecap
			}
			if vIncludePulse != nil {
				v.IncludePulse = *vIncludePulse
			}
			if vSortMode != nil {
				v.SortMode = *vSortMode
			}
			_ = json.Unmarshal(vTagIDsJSON, &v.TagIDs)
			_ = json.Unmarshal(vSourceIDsJSON, &v.SourceIDs)
			l.CurrentVersion = v
		}

		lenses = append(lenses, l)
	}
	return lenses, nil
}

// GetLens returns a lens by ID.
func (r *Repository) GetLens(ctx context.Context, lensID uuid.UUID) (*KnowledgeLens, error) {
	query := `SELECT lens_id, user_id, tenant_id, name, description, created_at, updated_at, archived_at
		FROM knowledge_lenses WHERE lens_id = $1`
	var l KnowledgeLens
	err := r.pool.QueryRow(ctx, query, lensID).Scan(
		&l.LensID, &l.UserID, &l.TenantID, &l.Name, &l.Description, &l.CreatedAt, &l.UpdatedAt, &l.ArchivedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetLens: %w", err)
	}
	return &l, nil
}

// GetCurrentLensVersion returns the non-superseded version of a lens.
func (r *Repository) GetCurrentLensVersion(ctx context.Context, lensID uuid.UUID) (*KnowledgeLensVersion, error) {
	query := `SELECT lens_version_id, lens_id, created_at, query_text,
		tag_ids_json, source_ids_json, time_window_json,
		include_recap, include_pulse, sort_mode, superseded_by
		FROM knowledge_lens_versions WHERE lens_id = $1 AND superseded_by IS NULL
		ORDER BY created_at DESC LIMIT 1`

	var v KnowledgeLensVersion
	var tagIDsJSON, sourceIDsJSON []byte
	var timeWindow *string
	err := r.pool.QueryRow(ctx, query, lensID).Scan(
		&v.LensVersionID, &v.LensID, &v.CreatedAt, &v.QueryText,
		&tagIDsJSON, &sourceIDsJSON, &timeWindow,
		&v.IncludeRecap, &v.IncludePulse, &v.SortMode, &v.SupersededBy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetCurrentLensVersion: %w", err)
	}
	_ = json.Unmarshal(tagIDsJSON, &v.TagIDs)
	_ = json.Unmarshal(sourceIDsJSON, &v.SourceIDs)
	if timeWindow != nil {
		v.TimeWindow = *timeWindow
	}
	return &v, nil
}

// GetCurrentLensSelection returns the user's current lens selection.
func (r *Repository) GetCurrentLensSelection(ctx context.Context, userID uuid.UUID) (*KnowledgeCurrentLens, error) {
	query := `SELECT user_id, lens_id, lens_version_id, selected_at
		FROM knowledge_current_lens WHERE user_id = $1`
	var c KnowledgeCurrentLens
	err := r.pool.QueryRow(ctx, query, userID).Scan(&c.UserID, &c.LensID, &c.LensVersionID, &c.SelectedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetCurrentLensSelection: %w", err)
	}
	return &c, nil
}

// SelectCurrentLens upserts the current lens selection.
func (r *Repository) SelectCurrentLens(ctx context.Context, c KnowledgeCurrentLens) error {
	query := `INSERT INTO knowledge_current_lens (user_id, lens_id, lens_version_id, selected_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET lens_id = $2, lens_version_id = $3, selected_at = $4`
	_, err := r.pool.Exec(ctx, query, c.UserID, c.LensID, c.LensVersionID, c.SelectedAt)
	if err != nil {
		return fmt.Errorf("SelectCurrentLens: %w", err)
	}
	return nil
}

// ClearCurrentLens removes the current lens selection.
func (r *Repository) ClearCurrentLens(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM knowledge_current_lens WHERE user_id = $1`
	_, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("ClearCurrentLens: %w", err)
	}
	return nil
}

// ArchiveLens marks a lens as archived and clears its selection.
func (r *Repository) ArchiveLens(ctx context.Context, lensID uuid.UUID) error {
	query := `UPDATE knowledge_lenses SET archived_at = now(), updated_at = now() WHERE lens_id = $1`
	_, err := r.pool.Exec(ctx, query, lensID)
	if err != nil {
		return fmt.Errorf("ArchiveLens: %w", err)
	}
	// Clear selection if this lens was selected
	clearQuery := `DELETE FROM knowledge_current_lens WHERE lens_id = $1`
	_, err = r.pool.Exec(ctx, clearQuery, lensID)
	if err != nil {
		return fmt.Errorf("ArchiveLens clear selection: %w", err)
	}
	return nil
}

// CreateLens inserts a new lens.
func (r *Repository) CreateLens(ctx context.Context, l KnowledgeLens) error {
	query := `INSERT INTO knowledge_lenses (lens_id, user_id, tenant_id, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, query, l.LensID, l.UserID, l.TenantID, l.Name, l.Description, l.CreatedAt, l.UpdatedAt)
	if err != nil {
		return fmt.Errorf("CreateLens: %w", err)
	}
	return nil
}

// CreateLensVersion inserts a new lens version.
func (r *Repository) CreateLensVersion(ctx context.Context, v KnowledgeLensVersion) error {
	tagIDsJSON, _ := json.Marshal(v.TagIDs)
	sourceIDsJSON, _ := json.Marshal(v.SourceIDs)
	query := `INSERT INTO knowledge_lens_versions
		(lens_version_id, lens_id, created_at, query_text, tag_ids_json, source_ids_json,
		 time_window_json, include_recap, include_pulse, sort_mode)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := r.pool.Exec(ctx, query,
		v.LensVersionID, v.LensID, v.CreatedAt, v.QueryText,
		string(tagIDsJSON), string(sourceIDsJSON), v.TimeWindow,
		v.IncludeRecap, v.IncludePulse, v.SortMode,
	)
	if err != nil {
		return fmt.Errorf("CreateLensVersion: %w", err)
	}
	return nil
}

// ResolveLensFilter resolves the lens filter for a user.
func (r *Repository) ResolveLensFilter(ctx context.Context, userID uuid.UUID, lensID *uuid.UUID) (*LensFilter, error) {
	var targetLensID uuid.UUID
	if lensID != nil {
		targetLensID = *lensID
	} else {
		sel, err := r.GetCurrentLensSelection(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("ResolveLensFilter: %w", err)
		}
		if sel == nil {
			return nil, nil
		}
		targetLensID = sel.LensID
	}

	lens, err := r.GetLens(ctx, targetLensID)
	if err != nil || lens == nil {
		return nil, err
	}

	version, err := r.GetCurrentLensVersion(ctx, targetLensID)
	if err != nil || version == nil {
		return nil, err
	}

	return &LensFilter{
		QueryText:    version.QueryText,
		TagNames:     version.TagIDs,
		SourceIDs:    version.SourceIDs,
		TimeWindow:   version.TimeWindow,
		IncludeRecap: version.IncludeRecap,
		IncludePulse: version.IncludePulse,
		SortMode:     version.SortMode,
	}, nil
}
