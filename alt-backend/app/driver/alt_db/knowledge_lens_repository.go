package alt_db

import (
	"alt/domain"
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	feedIDsJSON, _ := json.Marshal(version.FeedIDs)
	var timeWindowJSON []byte
	if version.TimeWindow != "" {
		timeWindowJSON, _ = json.Marshal(version.TimeWindow)
	}

	query := `INSERT INTO knowledge_lens_versions
		(lens_version_id, lens_id, created_at, query_text, tag_ids_json, feed_ids_json, time_window_json, include_recap, include_pulse, sort_mode)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := r.pool.Exec(ctx, query,
		version.LensVersionID, version.LensID, version.CreatedAt,
		version.QueryText, tagIDsJSON, feedIDsJSON, timeWindowJSON,
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

	query := `SELECT l.lens_id, l.user_id, l.tenant_id, l.name, l.description, l.created_at, l.updated_at,
		v.lens_version_id, v.created_at, v.query_text, v.tag_ids_json, v.feed_ids_json,
		v.time_window_json, v.include_recap, v.include_pulse, v.sort_mode, v.superseded_by
		FROM knowledge_lenses
		AS l
		LEFT JOIN LATERAL (
			SELECT lens_version_id, created_at, query_text, tag_ids_json, feed_ids_json,
				time_window_json, include_recap, include_pulse, sort_mode, superseded_by
			FROM knowledge_lens_versions
			WHERE lens_id = l.lens_id AND superseded_by IS NULL
			ORDER BY created_at DESC
			LIMIT 1
		) AS v ON true
		WHERE l.user_id = $1 AND l.archived_at IS NULL
		ORDER BY l.updated_at DESC`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("ListLenses: %w", err)
	}
	defer rows.Close()

	var lenses []domain.KnowledgeLens
	for rows.Next() {
		var l domain.KnowledgeLens
		var (
			version                                 domain.KnowledgeLensVersion
			versionID                               *uuid.UUID
			versionCreatedAt                        *time.Time
			queryText                               *string
			tagIDsJSON, feedIDsJSON, timeWindowJSON []byte
			includeRecap, includePulse              *bool
			sortMode                                *string
			supersededBy                            *uuid.UUID
		)
		if err := rows.Scan(
			&l.LensID, &l.UserID, &l.TenantID, &l.Name, &l.Description, &l.CreatedAt, &l.UpdatedAt,
			&versionID, &versionCreatedAt, &queryText, &tagIDsJSON, &feedIDsJSON,
			&timeWindowJSON, &includeRecap, &includePulse, &sortMode, &supersededBy,
		); err != nil {
			return nil, fmt.Errorf("ListLenses scan: %w", err)
		}
		if versionID != nil {
			version.LensVersionID = *versionID
			version.LensID = l.LensID
			if versionCreatedAt != nil {
				version.CreatedAt = *versionCreatedAt
			}
			if queryText != nil {
				version.QueryText = *queryText
			}
			_ = json.Unmarshal(tagIDsJSON, &version.TagIDs)
			_ = json.Unmarshal(feedIDsJSON, &version.FeedIDs)
			if timeWindowJSON != nil {
				_ = json.Unmarshal(timeWindowJSON, &version.TimeWindow)
			}
			if includeRecap != nil {
				version.IncludeRecap = *includeRecap
			}
			if includePulse != nil {
				version.IncludePulse = *includePulse
			}
			if sortMode != nil {
				version.SortMode = *sortMode
			}
			version.SupersededBy = supersededBy
			l.CurrentVersion = &version
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
		feed_ids_json, time_window_json, include_recap, include_pulse, sort_mode, superseded_by
		FROM knowledge_lens_versions
		WHERE lens_id = $1 AND superseded_by IS NULL
		ORDER BY created_at DESC LIMIT 1`

	var v domain.KnowledgeLensVersion
	var tagIDsJSON, feedIDsJSON, timeWindowJSON []byte
	err := r.pool.QueryRow(ctx, query, lensID).Scan(
		&v.LensVersionID, &v.LensID, &v.CreatedAt, &v.QueryText,
		&tagIDsJSON, &feedIDsJSON, &timeWindowJSON, &v.IncludeRecap, &v.IncludePulse, &v.SortMode, &v.SupersededBy,
	)
	if err != nil {
		return nil, fmt.Errorf("GetCurrentLensVersion: %w", err)
	}
	_ = json.Unmarshal(tagIDsJSON, &v.TagIDs)
	_ = json.Unmarshal(feedIDsJSON, &v.FeedIDs)
	if timeWindowJSON != nil {
		_ = json.Unmarshal(timeWindowJSON, &v.TimeWindow)
	}
	return &v, nil
}

func (r *AltDBRepository) GetCurrentLensSelection(ctx context.Context, userID uuid.UUID) (*domain.KnowledgeCurrentLens, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetCurrentLensSelection")
	defer span.End()

	query := `SELECT user_id, lens_id, lens_version_id, selected_at
		FROM knowledge_current_lens
		WHERE user_id = $1`

	var current domain.KnowledgeCurrentLens
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&current.UserID, &current.LensID, &current.LensVersionID, &current.SelectedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetCurrentLensSelection: %w", err)
	}
	return &current, nil
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

func (r *AltDBRepository) ClearCurrentLens(ctx context.Context, userID uuid.UUID) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ClearCurrentLens")
	defer span.End()

	query := `DELETE FROM knowledge_current_lens WHERE user_id = $1`
	if _, err := r.pool.Exec(ctx, query, userID); err != nil {
		return fmt.Errorf("ClearCurrentLens: %w", err)
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
	if _, err := r.pool.Exec(ctx, `DELETE FROM knowledge_current_lens WHERE lens_id = $1`, lensID); err != nil {
		return fmt.Errorf("ArchiveLens clear current: %w", err)
	}
	return nil
}

func (r *AltDBRepository) ResolveKnowledgeHomeLens(ctx context.Context, userID uuid.UUID, lensID *uuid.UUID) (*domain.KnowledgeHomeLensFilter, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ResolveKnowledgeHomeLens")
	defer span.End()

	targetLensID := lensID
	if targetLensID == nil || *targetLensID == uuid.Nil {
		current, err := r.GetCurrentLensSelection(ctx, userID)
		if err != nil {
			return nil, nil
		}
		targetLensID = &current.LensID
	}

	lens, err := r.GetLens(ctx, *targetLensID)
	if err != nil {
		return nil, nil
	}
	if lens.ArchivedAt != nil || lens.UserID != userID {
		return nil, nil
	}
	version, err := r.GetCurrentLensVersion(ctx, *targetLensID)
	if err != nil {
		return nil, fmt.Errorf("ResolveKnowledgeHomeLens version: %w", err)
	}

	filter := &domain.KnowledgeHomeLensFilter{
		LensID:     *targetLensID,
		TagNames:   append([]string(nil), version.TagIDs...),
		TimeWindow: version.TimeWindow,
	}
	for _, rawFeedID := range version.FeedIDs {
		feedID, err := uuid.Parse(rawFeedID)
		if err != nil {
			continue
		}
		filter.FeedIDs = append(filter.FeedIDs, feedID)
	}
	return filter, nil
}
