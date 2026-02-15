package alt_db

import (
	"context"
	"fmt"
	"time"
)

// InternalUnsummarizedArticle represents an article without a summary.
type InternalUnsummarizedArticle struct {
	ID        string
	Title     string
	Content   string
	URL       string
	CreatedAt time.Time
	UserID    string
}

// ListUnsummarizedArticles returns articles that have no entries in article_summaries,
// using backward keyset pagination (created_at DESC, id DESC).
func (r *AltDBRepository) ListUnsummarizedArticles(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]InternalUnsummarizedArticle, *time.Time, string, error) {
	var query string
	var args []any

	if lastCreatedAt == nil || lastCreatedAt.IsZero() {
		query = `
			SELECT a.id, a.title, a.content, a.url, a.created_at, a.user_id
			FROM articles a
			WHERE NOT EXISTS (SELECT 1 FROM article_summaries s WHERE s.article_id = a.id)
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $1
		`
		args = []any{limit}
	} else {
		query = `
			SELECT a.id, a.title, a.content, a.url, a.created_at, a.user_id
			FROM articles a
			WHERE NOT EXISTS (SELECT 1 FROM article_summaries s WHERE s.article_id = a.id)
			  AND (a.created_at, a.id) < ($1, $2)
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $3
		`
		args = []any{*lastCreatedAt, lastID, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, "", fmt.Errorf("list unsummarized articles: %w", err)
	}
	defer rows.Close()

	var articles []InternalUnsummarizedArticle
	var finalCreatedAt *time.Time
	var finalID string

	for rows.Next() {
		var a InternalUnsummarizedArticle
		if err := rows.Scan(&a.ID, &a.Title, &a.Content, &a.URL, &a.CreatedAt, &a.UserID); err != nil {
			return nil, nil, "", fmt.Errorf("scan unsummarized article: %w", err)
		}
		articles = append(articles, a)
		finalCreatedAt = &a.CreatedAt
		finalID = a.ID
	}

	if err = rows.Err(); err != nil {
		return nil, nil, "", fmt.Errorf("iterate unsummarized articles: %w", err)
	}

	return articles, finalCreatedAt, finalID, nil
}

// HasUnsummarizedArticles checks if there are any articles without summaries.
func (r *AltDBRepository) HasUnsummarizedArticles(ctx context.Context) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM articles a
			WHERE NOT EXISTS (SELECT 1 FROM article_summaries s WHERE s.article_id = a.id)
		)
	`
	var exists bool
	err := r.pool.QueryRow(ctx, query).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("has unsummarized articles: %w", err)
	}
	return exists, nil
}
