package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

// FetchArticlesByTag retrieves articles associated with a specific tag for the Tag Trail feature.
// Uses cursor-based pagination (by created_at) for efficient traversal.
func (r *AltDBRepository) FetchArticlesByTag(ctx context.Context, tagID string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	if r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	var query string
	var args []interface{}

	if cursor == nil {
		// First page - no cursor
		query = `
			SELECT a.id, a.title, a.url, a.created_at, a.feed_id, COALESCE(f.title, '') as feed_title
			FROM articles a
			INNER JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN feeds f ON a.feed_id = f.id
			WHERE at.feed_tag_id = $1
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $2
		`
		args = []interface{}{tagID, limit}
	} else {
		// Subsequent pages - use cursor
		query = `
			SELECT a.id, a.title, a.url, a.created_at, a.feed_id, COALESCE(f.title, '') as feed_title
			FROM articles a
			INNER JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN feeds f ON a.feed_id = f.id
			WHERE at.feed_tag_id = $1
			AND a.created_at < $2
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $3
		`
		args = []interface{}{tagID, cursor, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching articles by tag", "error", err, "tagID", tagID)
		return nil, errors.New("error fetching articles by tag")
	}
	defer rows.Close()

	var articles []*domain.TagTrailArticle
	for rows.Next() {
		var article domain.TagTrailArticle
		err := rows.Scan(&article.ID, &article.Title, &article.Link, &article.PublishedAt, &article.FeedID, &article.FeedTitle)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning article", "error", err)
			return nil, errors.New("error scanning articles by tag")
		}
		articles = append(articles, &article)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "row iteration error", "error", err)
		return nil, errors.New("error iterating articles by tag")
	}

	logger.Logger.InfoContext(ctx, "fetched articles by tag from database", "tagID", tagID, "count", len(articles))
	return articles, nil
}

// FetchArticlesByTagName retrieves articles associated with a specific tag name across all feeds.
// This allows searching for articles by tag name rather than a specific feed_tag_id,
// enabling cross-feed tag discovery for the Tag Trail feature.
func (r *AltDBRepository) FetchArticlesByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	if r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	var query string
	var args []interface{}

	if cursor == nil {
		// First page - no cursor
		query = `
			SELECT DISTINCT a.id, a.title, a.url, a.created_at, a.feed_id, COALESCE(f.title, '') as feed_title
			FROM articles a
			INNER JOIN article_tags at ON a.id = at.article_id
			INNER JOIN feed_tags ft ON at.feed_tag_id = ft.id
			LEFT JOIN feeds f ON a.feed_id = f.id
			WHERE ft.tag_name = $1
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $2
		`
		args = []interface{}{tagName, limit}
	} else {
		// Subsequent pages - use cursor
		query = `
			SELECT DISTINCT a.id, a.title, a.url, a.created_at, a.feed_id, COALESCE(f.title, '') as feed_title
			FROM articles a
			INNER JOIN article_tags at ON a.id = at.article_id
			INNER JOIN feed_tags ft ON at.feed_tag_id = ft.id
			LEFT JOIN feeds f ON a.feed_id = f.id
			WHERE ft.tag_name = $1
			AND a.created_at < $2
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $3
		`
		args = []interface{}{tagName, cursor, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching articles by tag name", "error", err, "tagName", tagName)
		return nil, errors.New("error fetching articles by tag name")
	}
	defer rows.Close()

	var articles []*domain.TagTrailArticle
	for rows.Next() {
		var article domain.TagTrailArticle
		err := rows.Scan(&article.ID, &article.Title, &article.Link, &article.PublishedAt, &article.FeedID, &article.FeedTitle)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning article", "error", err)
			return nil, errors.New("error scanning articles by tag name")
		}
		articles = append(articles, &article)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "row iteration error", "error", err)
		return nil, errors.New("error iterating articles by tag name")
	}

	logger.Logger.InfoContext(ctx, "fetched articles by tag name from database", "tagName", tagName, "count", len(articles))
	return articles, nil
}
