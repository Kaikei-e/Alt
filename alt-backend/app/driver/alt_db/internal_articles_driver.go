package alt_db

import (
	"context"
	"time"
)

// InternalArticleWithTags is the driver-level model for articles with tags.
type InternalArticleWithTags struct {
	ID        string
	Title     string
	Content   string
	Tags      []string
	CreatedAt time.Time
	UserID    string
}

// InternalDeletedArticle is the driver-level model for deleted articles.
type InternalDeletedArticle struct {
	ID        string
	DeletedAt time.Time
}

// ListArticlesWithTags fetches articles with tags using backward keyset pagination.
func (r *AltDBRepository) ListArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*InternalArticleWithTags, *time.Time, string, error) {
	var query string
	var args []interface{}

	if lastCreatedAt == nil || lastCreatedAt.IsZero() {
		query = `
			SELECT a.id, a.title, a.content, a.created_at, a.user_id,
				   COALESCE(tags.tag_names, '{}') as tag_names
			FROM (
				SELECT id, title, content, created_at, user_id
				FROM articles
				WHERE deleted_at IS NULL
				ORDER BY created_at DESC, id DESC
				LIMIT $1
			) a
			LEFT JOIN LATERAL (
				SELECT ARRAY_AGG(t.tag_name ORDER BY t.tag_name) as tag_names
				FROM article_tags at
				JOIN feed_tags t ON at.feed_tag_id = t.id
				WHERE at.article_id = a.id
			) tags ON TRUE
			ORDER BY a.created_at DESC, a.id DESC
		`
		args = []interface{}{limit}
	} else {
		query = `
			SELECT a.id, a.title, a.content, a.created_at, a.user_id,
				   COALESCE(tags.tag_names, '{}') as tag_names
			FROM (
				SELECT id, title, content, created_at, user_id
				FROM articles
				WHERE deleted_at IS NULL
				  AND (created_at, id) < ($1, $2)
				ORDER BY created_at DESC, id DESC
				LIMIT $3
			) a
			LEFT JOIN LATERAL (
				SELECT ARRAY_AGG(t.tag_name ORDER BY t.tag_name) as tag_names
				FROM article_tags at
				JOIN feed_tags t ON at.feed_tag_id = t.id
				WHERE at.article_id = a.id
			) tags ON TRUE
			ORDER BY a.created_at DESC, a.id DESC
		`
		args = []interface{}{*lastCreatedAt, lastID, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, "", err
	}
	defer rows.Close()

	var articles []*InternalArticleWithTags
	var finalCreatedAt *time.Time
	var finalID string

	for rows.Next() {
		var article InternalArticleWithTags
		var tagNames []string

		if err := rows.Scan(&article.ID, &article.Title, &article.Content, &article.CreatedAt, &article.UserID, &tagNames); err != nil {
			return nil, nil, "", err
		}

		article.Tags = filterEmptyStrings(tagNames)
		articles = append(articles, &article)
		finalCreatedAt = &article.CreatedAt
		finalID = article.ID
	}

	if err := rows.Err(); err != nil {
		return nil, nil, "", err
	}

	return articles, finalCreatedAt, finalID, nil
}

// ListArticlesWithTagsForward fetches articles with tags using forward keyset pagination.
func (r *AltDBRepository) ListArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*InternalArticleWithTags, *time.Time, string, error) {
	var query string
	var args []interface{}

	if lastCreatedAt == nil || lastCreatedAt.IsZero() {
		query = `
			SELECT a.id, a.title, a.content, a.created_at, a.user_id,
				   COALESCE(tags.tag_names, '{}') as tag_names
			FROM (
				SELECT id, title, content, created_at, user_id
				FROM articles
				WHERE deleted_at IS NULL
				  AND created_at > $1
				ORDER BY created_at ASC, id ASC
				LIMIT $2
			) a
			LEFT JOIN LATERAL (
				SELECT ARRAY_AGG(t.tag_name ORDER BY t.tag_name) as tag_names
				FROM article_tags at
				JOIN feed_tags t ON at.feed_tag_id = t.id
				WHERE at.article_id = a.id
			) tags ON TRUE
			ORDER BY a.created_at ASC, a.id ASC
		`
		args = []interface{}{*incrementalMark, limit}
	} else {
		query = `
			SELECT a.id, a.title, a.content, a.created_at, a.user_id,
				   COALESCE(tags.tag_names, '{}') as tag_names
			FROM (
				SELECT id, title, content, created_at, user_id
				FROM articles
				WHERE deleted_at IS NULL
				  AND created_at > $1
				  AND (created_at, id) > ($2, $3)
				ORDER BY created_at ASC, id ASC
				LIMIT $4
			) a
			LEFT JOIN LATERAL (
				SELECT ARRAY_AGG(t.tag_name ORDER BY t.tag_name) as tag_names
				FROM article_tags at
				JOIN feed_tags t ON at.feed_tag_id = t.id
				WHERE at.article_id = a.id
			) tags ON TRUE
			ORDER BY a.created_at ASC, a.id ASC
		`
		args = []interface{}{*incrementalMark, *lastCreatedAt, lastID, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, "", err
	}
	defer rows.Close()

	var articles []*InternalArticleWithTags
	var finalCreatedAt *time.Time
	var finalID string

	for rows.Next() {
		var article InternalArticleWithTags
		var tagNames []string

		if err := rows.Scan(&article.ID, &article.Title, &article.Content, &article.CreatedAt, &article.UserID, &tagNames); err != nil {
			return nil, nil, "", err
		}

		article.Tags = filterEmptyStrings(tagNames)
		articles = append(articles, &article)
		finalCreatedAt = &article.CreatedAt
		finalID = article.ID
	}

	if err := rows.Err(); err != nil {
		return nil, nil, "", err
	}

	return articles, finalCreatedAt, finalID, nil
}

// ListDeletedArticles fetches deleted articles for syncing deletions.
func (r *AltDBRepository) ListDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]*InternalDeletedArticle, *time.Time, error) {
	var query string
	var args []interface{}

	if lastDeletedAt == nil || lastDeletedAt.IsZero() {
		query = `
			SELECT id, deleted_at
			FROM articles
			WHERE deleted_at IS NOT NULL
			ORDER BY deleted_at ASC, id ASC
			LIMIT $1
		`
		args = []interface{}{limit}
	} else {
		query = `
			SELECT id, deleted_at
			FROM articles
			WHERE deleted_at IS NOT NULL
			  AND (deleted_at, id) > ($1, '')
			ORDER BY deleted_at ASC, id ASC
			LIMIT $2
		`
		args = []interface{}{*lastDeletedAt, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var deletedArticles []*InternalDeletedArticle
	var finalDeletedAt *time.Time

	for rows.Next() {
		var article InternalDeletedArticle
		if err := rows.Scan(&article.ID, &article.DeletedAt); err != nil {
			return nil, nil, err
		}
		deletedArticles = append(deletedArticles, &article)
		finalDeletedAt = &article.DeletedAt
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return deletedArticles, finalDeletedAt, nil
}

// GetLatestArticleTimestamp returns the latest created_at timestamp.
func (r *AltDBRepository) GetLatestArticleTimestamp(ctx context.Context) (*time.Time, error) {
	var latestCreatedAt *time.Time
	err := r.pool.QueryRow(ctx, `SELECT MAX(created_at) FROM articles WHERE deleted_at IS NULL`).Scan(&latestCreatedAt)
	if err != nil {
		return nil, err
	}
	return latestCreatedAt, nil
}

// GetArticleWithTagsByID retrieves a single article with tags by ID.
func (r *AltDBRepository) GetArticleWithTagsByID(ctx context.Context, articleID string) (*InternalArticleWithTags, error) {
	query := `
		SELECT a.id, a.title, a.content, a.created_at, a.user_id,
			   COALESCE(
				   array_agg(t.tag_name ORDER BY t.tag_name) FILTER (WHERE t.tag_name IS NOT NULL),
				   '{}'
			   ) as tag_names
		FROM articles a
		LEFT JOIN article_tags at ON a.id = at.article_id
		LEFT JOIN feed_tags t ON at.feed_tag_id = t.id
		WHERE a.id = $1 AND a.deleted_at IS NULL
		GROUP BY a.id
	`

	var article InternalArticleWithTags
	var tagNames []string

	err := r.pool.QueryRow(ctx, query, articleID).Scan(
		&article.ID, &article.Title, &article.Content, &article.CreatedAt, &article.UserID, &tagNames,
	)
	if err != nil {
		return nil, err
	}

	article.Tags = filterEmptyStrings(tagNames)
	return &article, nil
}

func filterEmptyStrings(ss []string) []string {
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}
