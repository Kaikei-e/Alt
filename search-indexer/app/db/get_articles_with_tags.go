package db

import (
	"context"
	"time"

	"search-indexer/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GetArticlesWithTags retrieves articles with their associated tags using cursor-based pagination
// lastCreatedAt and lastID are used as cursor parameters for pagination
// Returns: articles with tags, lastCreatedAt, lastID, error for cursor tracking
func GetArticlesWithTags(ctx context.Context, db *pgxpool.Pool, lastCreatedAt *time.Time, lastID string, limit int) ([]*models.Article, *time.Time, string, error) {
	var articles []*models.Article
	var finalCreatedAt *time.Time
	var finalID string

	var query string
	var args []interface{}

	if lastCreatedAt == nil || lastCreatedAt.IsZero() {
		// First query - no cursor constraint
		query = `
			SELECT a.id, a.title, a.content, a.url, a.created_at,
				   COALESCE(
					   array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),
					   '{}'
				   ) as tag_names
			FROM articles a
			LEFT JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN tags t ON at.tag_id = t.id
			GROUP BY a.id, a.title, a.content, a.url, a.created_at
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $1
		`
		args = []interface{}{limit}
	} else {
		// Subsequent queries - use efficient keyset pagination
		query = `
			SELECT a.id, a.title, a.content, a.url, a.created_at,
				   COALESCE(
					   array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),
					   '{}'
				   ) as tag_names
			FROM articles a
			LEFT JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN tags t ON at.tag_id = t.id
			WHERE (a.created_at, a.id) < ($1, $2)
			GROUP BY a.id, a.title, a.content, a.url, a.created_at
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $3
		`
		args = []interface{}{*lastCreatedAt, lastID, limit}
	}

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, "", err
	}
	defer rows.Close()

	for rows.Next() {
		var article models.Article
		var tagNames []string

		err = rows.Scan(&article.ID, &article.Title, &article.Content, &article.URL, &article.CreatedAt, &tagNames)
		if err != nil {
			return nil, nil, "", err
		}

		// Convert tag names to Tag structs for consistency
		var tags []models.Tag
		for _, tagName := range tagNames {
			if tagName != "" {
				tags = append(tags, models.Tag{Name: tagName})
			}
		}
		article.Tags = tags

		articles = append(articles, &article)
		// Keep track of the last item for cursor
		finalCreatedAt = &article.CreatedAt
		finalID = article.ID
	}

	if err = rows.Err(); err != nil {
		return nil, nil, "", err
	}

	return articles, finalCreatedAt, finalID, nil
}

// GetArticlesWithTagsCount gets the total count of articles with tags
func GetArticlesWithTagsCount(ctx context.Context, db *pgxpool.Pool) (int, error) {
	query := `SELECT COUNT(*) FROM articles`

	var count int
	err := db.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}
