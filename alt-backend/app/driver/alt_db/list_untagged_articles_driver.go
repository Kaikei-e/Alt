package alt_db

import (
	"context"
	"fmt"
	"time"
)

// InternalUntaggedArticle represents an article without tags.
type InternalUntaggedArticle struct {
	ID        string
	Title     string
	Content   string
	UserID    string
	FeedID    *string
	CreatedAt time.Time
}

// ListUntaggedArticles returns articles that have no entries in article_tags,
// using backward keyset pagination (created_at DESC, id DESC).
// Articles with NULL feed_id are excluded since they cannot receive tags.
func (r *AltDBRepository) ListUntaggedArticles(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]InternalUntaggedArticle, *time.Time, string, int32, error) {
	// Count total untagged articles (excluding NULL feed_id)
	var totalCount int32
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM articles a
		WHERE NOT EXISTS (
			SELECT 1 FROM article_tags at WHERE at.article_id = a.id
		)
		AND a.feed_id IS NOT NULL
	`).Scan(&totalCount)
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("count untagged: %w", err)
	}

	var query string
	var args []any

	if lastCreatedAt == nil || lastCreatedAt.IsZero() {
		query = `
			SELECT a.id, a.title, a.content, a.user_id, a.feed_id::text, a.created_at
			FROM articles a
			WHERE NOT EXISTS (SELECT 1 FROM article_tags at WHERE at.article_id = a.id)
			  AND a.feed_id IS NOT NULL
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $1
		`
		args = []any{limit}
	} else {
		query = `
			SELECT a.id, a.title, a.content, a.user_id, a.feed_id::text, a.created_at
			FROM articles a
			WHERE NOT EXISTS (SELECT 1 FROM article_tags at WHERE at.article_id = a.id)
			  AND a.feed_id IS NOT NULL
			  AND (a.created_at, a.id) < ($1, $2)
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $3
		`
		args = []any{*lastCreatedAt, lastID, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("list untagged: %w", err)
	}
	defer rows.Close()

	var articles []InternalUntaggedArticle
	var nextCreatedAt *time.Time
	var nextID string

	for rows.Next() {
		var a InternalUntaggedArticle
		if err := rows.Scan(&a.ID, &a.Title, &a.Content, &a.UserID, &a.FeedID, &a.CreatedAt); err != nil {
			return nil, nil, "", 0, fmt.Errorf("scan untagged: %w", err)
		}
		articles = append(articles, a)
		nextCreatedAt = &a.CreatedAt
		nextID = a.ID
	}

	if err = rows.Err(); err != nil {
		return nil, nil, "", 0, fmt.Errorf("iterate untagged: %w", err)
	}

	return articles, nextCreatedAt, nextID, totalCount, nil
}
