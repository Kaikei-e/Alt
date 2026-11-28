package sovereign_db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	defaultReadingAnnotationsLimit = 100
	maxReadingAnnotationsLimit     = 500
	defaultThemeCurationsLimit     = 200
	maxThemeCurationsLimit         = 500
)

// ReadingAnnotation represents one row in the reading_annotations read model.
type ReadingAnnotation struct {
	UserID       uuid.UUID
	TenantID     uuid.UUID
	AnnotationID uuid.UUID
	ArticleID    uuid.UUID
	Stance       string
	Excerpt      string
	Note         string
	CreatedAt    time.Time
	DeletedAt    *time.Time
	LastEventSeq int64
}

// ReadingThemeCuration represents one row in the reading_theme_curations read model.
type ReadingThemeCuration struct {
	UserID              uuid.UUID
	TenantID            uuid.UUID
	CurationID          uuid.UUID
	ThemeID             uuid.UUID
	ActionKind          string
	TargetHash          string
	TargetText          string
	UserNote            string
	AugurConversationID string
	CreatedAt           time.Time
	DeletedAt           *time.Time
	LastEventSeq        int64
}

// UpsertReadingAnnotation folds a reading.annotation_created.v1 event into
// reading_annotations idempotently (guarded by last_event_seq).
func (r *Repository) UpsertReadingAnnotation(ctx context.Context, ann ReadingAnnotation) error {
	const q = `
INSERT INTO reading_annotations (
  user_id, tenant_id, annotation_id, article_id, stance, excerpt, note, created_at, deleted_at, last_event_seq
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, $9)
ON CONFLICT (user_id, annotation_id) DO UPDATE SET
  stance         = EXCLUDED.stance,
  excerpt        = EXCLUDED.excerpt,
  note           = EXCLUDED.note,
  created_at     = EXCLUDED.created_at,
  deleted_at     = COALESCE(reading_annotations.deleted_at, EXCLUDED.deleted_at),
  last_event_seq = EXCLUDED.last_event_seq
WHERE reading_annotations.last_event_seq <= EXCLUDED.last_event_seq`
	if _, err := r.pool.Exec(ctx, q,
		ann.UserID, ann.TenantID, ann.AnnotationID, ann.ArticleID,
		ann.Stance, ann.Excerpt, ann.Note, ann.CreatedAt, ann.LastEventSeq,
	); err != nil {
		return fmt.Errorf("UpsertReadingAnnotation: %w", err)
	}
	return nil
}

// DeleteReadingAnnotation soft-deletes a reading_annotations row monotonically
// by last_event_seq so out-of-order replay stays deterministic.
func (r *Repository) DeleteReadingAnnotation(ctx context.Context, userID, annotationID uuid.UUID, deletedAt time.Time, eventSeq int64) error {
	const q = `
UPDATE reading_annotations
SET deleted_at     = $3,
    last_event_seq = $4
WHERE user_id = $1
  AND annotation_id = $2
  AND last_event_seq <= $4`
	if _, err := r.pool.Exec(ctx, q, userID, annotationID, deletedAt, eventSeq); err != nil {
		return fmt.Errorf("DeleteReadingAnnotation: %w", err)
	}
	return nil
}

// ListReadingAnnotations returns active annotations owned by userID, optionally
// scoped to articleIDs, ordered newest first.
func (r *Repository) ListReadingAnnotations(ctx context.Context, userID uuid.UUID, articleIDs []uuid.UUID, limit int) ([]ReadingAnnotation, error) {
	if limit <= 0 {
		limit = defaultReadingAnnotationsLimit
	} else if limit > maxReadingAnnotationsLimit {
		limit = maxReadingAnnotationsLimit
	}

	var (
		query string
		args  []any
	)
	if len(articleIDs) > 0 {
		query = `
SELECT user_id, tenant_id, annotation_id, article_id, stance, excerpt, note, created_at, last_event_seq
FROM reading_annotations
WHERE user_id = $1
  AND deleted_at IS NULL
  AND article_id = ANY($2::uuid[])
ORDER BY created_at DESC, annotation_id ASC
LIMIT $3`
		args = []any{userID, articleIDs, limit}
	} else {
		query = `
SELECT user_id, tenant_id, annotation_id, article_id, stance, excerpt, note, created_at, last_event_seq
FROM reading_annotations
WHERE user_id = $1
  AND deleted_at IS NULL
ORDER BY created_at DESC, annotation_id ASC
LIMIT $2`
		args = []any{userID, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListReadingAnnotations: %w", err)
	}
	defer rows.Close()

	var result []ReadingAnnotation
	for rows.Next() {
		var a ReadingAnnotation
		if err := rows.Scan(
			&a.UserID, &a.TenantID, &a.AnnotationID, &a.ArticleID,
			&a.Stance, &a.Excerpt, &a.Note, &a.CreatedAt, &a.LastEventSeq,
		); err != nil {
			return nil, fmt.Errorf("ListReadingAnnotations scan: %w", err)
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListReadingAnnotations rows: %w", err)
	}
	return result, nil
}

// UpsertReadingThemeCuration folds a reading.theme_item_curated.v1 event into
// reading_theme_curations idempotently (guarded by last_event_seq).
func (r *Repository) UpsertReadingThemeCuration(ctx context.Context, cur ReadingThemeCuration) error {
	const q = `
INSERT INTO reading_theme_curations (
  user_id, tenant_id, curation_id, theme_id, action_kind, target_hash, target_text, user_note, augur_conversation_id, created_at, deleted_at, last_event_seq
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULL, $11)
ON CONFLICT (user_id, curation_id) DO UPDATE SET
  action_kind           = EXCLUDED.action_kind,
  target_hash           = EXCLUDED.target_hash,
  target_text           = EXCLUDED.target_text,
  user_note             = EXCLUDED.user_note,
  augur_conversation_id = EXCLUDED.augur_conversation_id,
  created_at            = EXCLUDED.created_at,
  deleted_at            = COALESCE(reading_theme_curations.deleted_at, EXCLUDED.deleted_at),
  last_event_seq        = EXCLUDED.last_event_seq
WHERE reading_theme_curations.last_event_seq <= EXCLUDED.last_event_seq`
	if _, err := r.pool.Exec(ctx, q,
		cur.UserID, cur.TenantID, cur.CurationID, cur.ThemeID,
		cur.ActionKind, cur.TargetHash, cur.TargetText, cur.UserNote,
		cur.AugurConversationID, cur.CreatedAt, cur.LastEventSeq,
	); err != nil {
		return fmt.Errorf("UpsertReadingThemeCuration: %w", err)
	}
	return nil
}

// DeleteReadingThemeCuration soft-deletes a reading_theme_curations row
// monotonically by last_event_seq.
func (r *Repository) DeleteReadingThemeCuration(ctx context.Context, userID, curationID uuid.UUID, deletedAt time.Time, eventSeq int64) error {
	const q = `
UPDATE reading_theme_curations
SET deleted_at     = $3,
    last_event_seq = $4
WHERE user_id = $1
  AND curation_id = $2
  AND last_event_seq <= $4`
	if _, err := r.pool.Exec(ctx, q, userID, curationID, deletedAt, eventSeq); err != nil {
		return fmt.Errorf("DeleteReadingThemeCuration: %w", err)
	}
	return nil
}

// ListReadingThemeCurations returns active curations owned by userID for themeID,
// ordered newest first.
func (r *Repository) ListReadingThemeCurations(ctx context.Context, userID, themeID uuid.UUID, limit int) ([]ReadingThemeCuration, error) {
	if limit <= 0 {
		limit = defaultThemeCurationsLimit
	} else if limit > maxThemeCurationsLimit {
		limit = maxThemeCurationsLimit
	}

	const q = `
SELECT user_id, tenant_id, curation_id, theme_id, action_kind, target_hash, target_text, user_note, augur_conversation_id, created_at, last_event_seq
FROM reading_theme_curations
WHERE user_id = $1
  AND theme_id = $2
  AND deleted_at IS NULL
ORDER BY created_at DESC, curation_id ASC
LIMIT $3`

	rows, err := r.pool.Query(ctx, q, userID, themeID, limit)
	if err != nil {
		return nil, fmt.Errorf("ListReadingThemeCurations: %w", err)
	}
	defer rows.Close()

	var result []ReadingThemeCuration
	for rows.Next() {
		var c ReadingThemeCuration
		if err := rows.Scan(
			&c.UserID, &c.TenantID, &c.CurationID, &c.ThemeID,
			&c.ActionKind, &c.TargetHash, &c.TargetText, &c.UserNote,
			&c.AugurConversationID, &c.CreatedAt, &c.LastEventSeq,
		); err != nil {
			return nil, fmt.Errorf("ListReadingThemeCurations scan: %w", err)
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListReadingThemeCurations rows: %w", err)
	}
	return result, nil
}
