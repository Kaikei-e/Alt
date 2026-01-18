package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// FetchRecentArticles retrieves articles published within the specified time window
// This is used by rag-orchestrator for temporal topics feature
// Note: This is a system-level query that doesn't filter by user
// When limit=0, all articles within the time window are returned (no LIMIT clause)
func (r *AltDBRepository) FetchRecentArticles(ctx context.Context, since time.Time, limit int) ([]*domain.Article, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	// limit <= 0 means no limit (time constraint only)
	// limit > 500 is capped at 500
	if limit > 500 {
		limit = 500
	}

	var query string
	var rows pgx.Rows
	var err error

	if limit <= 0 {
		// No limit - return all articles within time window
		query = `
			SELECT
				a.id,
				a.feed_id,
				a.title,
				a.url,
				a.content,
				a.created_at as published_at,
				a.created_at,
				COALESCE(ARRAY_AGG(ft.tag_name) FILTER (WHERE ft.tag_name IS NOT NULL), '{}') as tags
			FROM articles a
			LEFT JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN feed_tags ft ON at.feed_tag_id = ft.id
			WHERE a.created_at >= $1
			GROUP BY a.id, a.feed_id, a.title, a.url, a.content, a.created_at
			ORDER BY a.created_at DESC, a.id DESC
		`
		rows, err = r.pool.Query(ctx, query, since)
	} else {
		// Apply limit
		query = `
			SELECT
				a.id,
				a.feed_id,
				a.title,
				a.url,
				a.content,
				a.created_at as published_at,
				a.created_at,
				COALESCE(ARRAY_AGG(ft.tag_name) FILTER (WHERE ft.tag_name IS NOT NULL), '{}') as tags
			FROM articles a
			LEFT JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN feed_tags ft ON at.feed_tag_id = ft.id
			WHERE a.created_at >= $1
			GROUP BY a.id, a.feed_id, a.title, a.url, a.content, a.created_at
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $2
		`
		rows, err = r.pool.Query(ctx, query, since, limit)
	}

	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching recent articles", "error", err, "since", since, "limit", limit)
		return nil, errors.New("error fetching recent articles")
	}
	defer rows.Close()

	var articles []*domain.Article
	for rows.Next() {
		var article domain.Article
		var tags []string

		err := rows.Scan(
			&article.ID,
			&article.FeedID,
			&article.Title,
			&article.URL,
			&article.Content,
			&article.PublishedAt,
			&article.CreatedAt,
			&tags,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				logger.Logger.InfoContext(ctx, "no recent articles found", "since", since)
				return articles, nil
			}
			logger.Logger.ErrorContext(ctx, "error scanning recent article", "error", err)
			return nil, errors.New("error scanning recent articles")
		}

		article.Tags = tags
		article.UpdatedAt = article.CreatedAt
		articles = append(articles, &article)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating recent articles rows", "error", err)
		return nil, errors.New("error processing recent articles")
	}

	logger.Logger.InfoContext(ctx, "fetched recent articles", "count", len(articles), "since", since)
	return articles, nil
}
