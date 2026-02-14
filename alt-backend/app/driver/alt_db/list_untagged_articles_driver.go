package alt_db

import (
	"context"
	"fmt"
)

// InternalUntaggedArticle represents an article without tags.
type InternalUntaggedArticle struct {
	ID      string
	Title   string
	Content string
	UserID  string
}

// ListUntaggedArticles returns articles that have no entries in article_tags.
func (r *AltDBRepository) ListUntaggedArticles(ctx context.Context, limit int, offset int) ([]InternalUntaggedArticle, int32, error) {
	// Count total untagged articles
	var totalCount int32
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM articles a
		WHERE NOT EXISTS (
			SELECT 1 FROM article_tags at WHERE at.article_id = a.id
		)
	`).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("count untagged: %w", err)
	}

	// Fetch paginated results
	rows, err := r.pool.Query(ctx, `
		SELECT a.id, a.title, a.content, a.user_id
		FROM articles a
		WHERE NOT EXISTS (
			SELECT 1 FROM article_tags at WHERE at.article_id = a.id
		)
		ORDER BY a.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list untagged: %w", err)
	}
	defer rows.Close()

	var articles []InternalUntaggedArticle
	for rows.Next() {
		var a InternalUntaggedArticle
		if err := rows.Scan(&a.ID, &a.Title, &a.Content, &a.UserID); err != nil {
			return nil, 0, fmt.Errorf("scan untagged: %w", err)
		}
		articles = append(articles, a)
	}

	return articles, totalCount, nil
}
