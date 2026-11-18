package sovereign_db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	defaultReadingThemesLimit = 100
	maxReadingThemesLimit     = 500
)

// ReadingTheme represents one row in the reading_themes read model.
type ReadingTheme struct {
	UserID            uuid.UUID
	TenantID          uuid.UUID
	ThemeID           uuid.UUID
	Title             string
	LatestVersion     int
	ArticleCount      int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastOpenedAt      *time.Time
	LastOpenedVersion int
}

// UpsertReadingThemeVersion folds a theme version snapshot into reading_themes
// and reading_theme_members inside a single transaction.
//
// Older versions arriving out of order leave both tables untouched. When the
// incoming version is >= the stored latest_version, reading_themes preserves
// created_at and last_opened_* while updating summary fields, and
// reading_theme_members converges to memberArticleIDs without overwriting
// added_in_version on retained members.
func (r *Repository) UpsertReadingThemeVersion(ctx context.Context, theme ReadingTheme, memberArticleIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("UpsertReadingThemeVersion begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit has succeeded

	const upsertThemeQuery = `
INSERT INTO reading_themes (
  user_id, tenant_id, theme_id, title, latest_version, article_count, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (user_id, theme_id) DO UPDATE SET
  title          = EXCLUDED.title,
  latest_version = EXCLUDED.latest_version,
  article_count  = EXCLUDED.article_count,
  updated_at     = EXCLUDED.updated_at
WHERE reading_themes.latest_version <= EXCLUDED.latest_version`

	tag, err := tx.Exec(ctx, upsertThemeQuery,
		theme.UserID, theme.TenantID, theme.ThemeID, theme.Title,
		theme.LatestVersion, theme.ArticleCount, theme.CreatedAt, theme.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("UpsertReadingThemeVersion upsert theme: %w", err)
	}

	if tag.RowsAffected() > 0 {
		if len(memberArticleIDs) == 0 {
			const deleteAllQuery = `
DELETE FROM reading_theme_members
WHERE user_id = $1
  AND theme_id = $2`
			if _, err := tx.Exec(ctx, deleteAllQuery, theme.UserID, theme.ThemeID); err != nil {
				return fmt.Errorf("UpsertReadingThemeVersion clear members: %w", err)
			}
		} else {
			const deleteRemovedQuery = `
DELETE FROM reading_theme_members
WHERE user_id = $1
  AND theme_id = $2
  AND NOT (article_id = ANY($3::uuid[]))`
			if _, err := tx.Exec(ctx, deleteRemovedQuery, theme.UserID, theme.ThemeID, memberArticleIDs); err != nil {
				return fmt.Errorf("UpsertReadingThemeVersion prune members: %w", err)
			}

			const insertMembersQuery = `
INSERT INTO reading_theme_members (user_id, tenant_id, theme_id, article_id, added_in_version)
SELECT $1, $2, $3, m.article_id, $4
FROM unnest($5::uuid[]) AS m(article_id)
ON CONFLICT (user_id, theme_id, article_id) DO NOTHING`
			if _, err := tx.Exec(ctx, insertMembersQuery,
				theme.UserID, theme.TenantID, theme.ThemeID, theme.LatestVersion, memberArticleIDs,
			); err != nil {
				return fmt.Errorf("UpsertReadingThemeVersion insert members: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("UpsertReadingThemeVersion commit: %w", err)
	}
	return nil
}

// MarkReadingThemeOpened records the latest opened timestamp and version
// monotonically. If the theme row does not exist yet, this is a no-op.
func (r *Repository) MarkReadingThemeOpened(ctx context.Context, userID, themeID uuid.UUID, openedAt time.Time, version int) error {
	const q = `
UPDATE reading_themes
SET last_opened_at = CASE
      WHEN last_opened_at IS NULL THEN $3
      ELSE GREATEST(last_opened_at, $3)
    END,
    last_opened_version = GREATEST(last_opened_version, $4)
WHERE user_id = $1
  AND theme_id = $2`
	if _, err := r.pool.Exec(ctx, q, userID, themeID, openedAt, version); err != nil {
		return fmt.Errorf("MarkReadingThemeOpened: %w", err)
	}
	return nil
}

// ListReadingThemes returns reading themes for a user ordered by updated_at
// descending and theme_id ascending.
func (r *Repository) ListReadingThemes(ctx context.Context, userID uuid.UUID, limit int) ([]ReadingTheme, error) {
	if limit <= 0 {
		limit = defaultReadingThemesLimit
	} else if limit > maxReadingThemesLimit {
		limit = maxReadingThemesLimit
	}

	const q = `
SELECT user_id, tenant_id, theme_id, title, latest_version, article_count,
       created_at, updated_at, last_opened_at, last_opened_version
FROM reading_themes
WHERE user_id = $1
ORDER BY updated_at DESC, theme_id ASC
LIMIT $2`
	rows, err := r.pool.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("ListReadingThemes: %w", err)
	}
	defer rows.Close()

	var themes []ReadingTheme
	for rows.Next() {
		var th ReadingTheme
		if err := rows.Scan(
			&th.UserID, &th.TenantID, &th.ThemeID, &th.Title,
			&th.LatestVersion, &th.ArticleCount,
			&th.CreatedAt, &th.UpdatedAt, &th.LastOpenedAt, &th.LastOpenedVersion,
		); err != nil {
			return nil, fmt.Errorf("ListReadingThemes scan: %w", err)
		}
		themes = append(themes, th)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListReadingThemes rows: %w", err)
	}
	return themes, nil
}
