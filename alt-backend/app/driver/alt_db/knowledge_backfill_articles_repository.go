package alt_db

import (
	"alt/domain"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CountBackfillArticles returns the number of non-deleted articles available for replay.
func (r *AltDBRepository) CountBackfillArticles(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM articles WHERE deleted_at IS NULL`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountBackfillArticles: %w", err)
	}
	return count, nil
}

// ListBackfillArticles returns a batch of historical articles ordered by created_at ASC, id ASC.
func (r *AltDBRepository) ListBackfillArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if lastCreatedAt == nil || lastArticleID == nil {
		rows, err = r.pool.Query(ctx, `
			SELECT id, user_id, created_at, COALESCE(published_at, created_at) AS published_at, title, COALESCE(url, '') AS url
			FROM articles
			WHERE deleted_at IS NULL
			ORDER BY created_at ASC, id ASC
			LIMIT $1
		`, limit)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, user_id, created_at, COALESCE(published_at, created_at) AS published_at, title, COALESCE(url, '') AS url
			FROM articles
			WHERE deleted_at IS NULL
			  AND (created_at, id) > ($1, $2)
			ORDER BY created_at ASC, id ASC
			LIMIT $3
		`, *lastCreatedAt, *lastArticleID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("ListBackfillArticles: %w", err)
	}
	defer rows.Close()

	articles := make([]domain.KnowledgeBackfillArticle, 0, limit)
	for rows.Next() {
		var article domain.KnowledgeBackfillArticle
		if err := rows.Scan(&article.ArticleID, &article.UserID, &article.CreatedAt, &article.PublishedAt, &article.Title, &article.URL); err != nil {
			return nil, fmt.Errorf("ListBackfillArticles scan: %w", err)
		}
		articles = append(articles, article)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListBackfillArticles rows: %w", err)
	}

	return articles, nil
}
