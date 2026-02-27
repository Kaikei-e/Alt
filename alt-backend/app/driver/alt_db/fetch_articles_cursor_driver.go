package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ArticleWithTags represents an article with its associated tags
type ArticleWithTags struct {
	ID          uuid.UUID `db:"id"`
	Title       string    `db:"title"`
	URL         string    `db:"url"`
	Content     string    `db:"content"`
	PublishedAt time.Time `db:"published_at"`
	CreatedAt   time.Time `db:"created_at"`
	Tags        []string  `db:"tags"`
}

// FetchArticlesWithCursor retrieves articles using cursor-based pagination
// Includes tags from tag-generator via article_tags and tags tables
func (r *AltDBRepository) FetchArticlesWithCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.Article, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	var query string
	var args []interface{}

	if cursor == nil {
		// First page - no cursor
		query = `
			SELECT
				a.id,
				a.title,
				a.url,
				a.content,
				a.created_at as published_at,
				a.created_at,
				COALESCE(tags.tag_names, '{}') as tags
			FROM (
				SELECT id, title, url, content, created_at
				FROM articles
				WHERE user_id = $1 AND deleted_at IS NULL
				ORDER BY created_at DESC, id DESC
				LIMIT $2
			) a
			LEFT JOIN LATERAL (
				SELECT ARRAY_AGG(ft.tag_name) as tag_names
				FROM article_tags at
				JOIN feed_tags ft ON at.feed_tag_id = ft.id
				WHERE at.article_id = a.id
			) tags ON TRUE
			ORDER BY a.created_at DESC, a.id DESC
		`
		args = []interface{}{user.UserID, limit}
	} else {
		// Subsequent pages - use cursor
		query = `
			SELECT
				a.id,
				a.title,
				a.url,
				a.content,
				a.created_at as published_at,
				a.created_at,
				COALESCE(tags.tag_names, '{}') as tags
			FROM (
				SELECT id, title, url, content, created_at
				FROM articles
				WHERE user_id = $1 AND deleted_at IS NULL
				AND created_at < $2
				ORDER BY created_at DESC, id DESC
				LIMIT $3
			) a
			LEFT JOIN LATERAL (
				SELECT ARRAY_AGG(ft.tag_name) as tag_names
				FROM article_tags at
				JOIN feed_tags ft ON at.feed_tag_id = ft.id
				WHERE at.article_id = a.id
			) tags ON TRUE
			ORDER BY a.created_at DESC, a.id DESC
		`
		args = []interface{}{user.UserID, cursor, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching articles with cursor", "error", err, "cursor", cursor, "user_id", user.UserID)
		return nil, errors.New("error fetching articles list")
	}
	defer rows.Close()

	var articles []*domain.Article
	for rows.Next() {
		var article domain.Article
		var tags []string

		err := rows.Scan(
			&article.ID,
			&article.Title,
			&article.URL,
			&article.Content,
			&article.PublishedAt,
			&article.CreatedAt,
			&tags,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				logger.Logger.InfoContext(ctx, "no articles found", "user_id", user.UserID)
				return articles, nil
			}
			logger.Logger.ErrorContext(ctx, "error scanning article with cursor", "error", err)
			return nil, errors.New("error scanning articles list")
		}

		article.Tags = tags
		articles = append(articles, &article)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating articles rows", "error", err)
		return nil, errors.New("error processing articles list")
	}

	logger.Logger.InfoContext(ctx, "fetched articles with cursor", "count", len(articles), "user_id", user.UserID)
	return articles, nil
}
