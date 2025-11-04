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
		logger.Logger.Error("user context not found", "error", err)
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
				COALESCE(ARRAY_AGG(ft.tag_name) FILTER (WHERE ft.tag_name IS NOT NULL), '{}') as tags
			FROM articles a
			LEFT JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN feed_tags ft ON at.feed_tag_id = ft.id
			WHERE a.user_id = $1
			GROUP BY a.id, a.title, a.url, a.content, a.created_at
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $2
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
				COALESCE(ARRAY_AGG(ft.tag_name) FILTER (WHERE ft.tag_name IS NOT NULL), '{}') as tags
			FROM articles a
			LEFT JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN feed_tags ft ON at.feed_tag_id = ft.id
			WHERE a.user_id = $1
			AND a.created_at < $2
			GROUP BY a.id, a.title, a.url, a.content, a.created_at
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $3
		`
		args = []interface{}{user.UserID, cursor, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.Error("error fetching articles with cursor", "error", err, "cursor", cursor, "user_id", user.UserID)
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
				logger.Logger.Info("no articles found", "user_id", user.UserID)
				return articles, nil
			}
			logger.Logger.Error("error scanning article with cursor", "error", err)
			return nil, errors.New("error scanning articles list")
		}

		article.Tags = tags
		articles = append(articles, &article)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("error iterating articles rows", "error", err)
		return nil, errors.New("error processing articles list")
	}

	logger.Logger.Info("fetched articles with cursor", "count", len(articles), "user_id", user.UserID)
	return articles, nil
}
