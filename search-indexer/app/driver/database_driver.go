package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"search-indexer/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DatabaseDriver struct {
	pool *pgxpool.Pool
}

func NewDatabaseDriver(pool *pgxpool.Pool) *DatabaseDriver {
	return &DatabaseDriver{
		pool: pool,
	}
}

// NewDatabaseDriverFromConfig creates a new DatabaseDriver with database connection
// constructed from environment variables
func NewDatabaseDriverFromConfig(ctx context.Context) (*DatabaseDriver, error) {
	pool, err := initDatabasePool(ctx)
	if err != nil {
		return nil, err
	}

	return &DatabaseDriver{
		pool: pool,
	}, nil
}

// initDatabasePool initializes the database connection pool
func initDatabasePool(ctx context.Context) (*pgxpool.Pool, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Construct DATABASE_URL from individual environment variables
		dbHost := os.Getenv("DB_HOST")
		dbPort := os.Getenv("DB_PORT")
		dbName := os.Getenv("DB_NAME")
		dbUser := os.Getenv("SEARCH_INDEXER_DB_USER")
		dbPassword := os.Getenv("SEARCH_INDEXER_DB_PASSWORD")

		if dbHost == "" || dbPort == "" || dbName == "" || dbUser == "" || dbPassword == "" {
			return nil, &DriverError{
				Op:  "initDatabasePool",
				Err: "database connection parameters are not set. Required: DB_HOST, DB_PORT, DB_NAME, SEARCH_INDEXER_DB_USER, SEARCH_INDEXER_DB_PASSWORD",
			}
		}

		dbURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPassword, dbHost, dbPort, dbName)
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, &DriverError{
			Op:  "initDatabasePool",
			Err: "failed to parse database URL: " + err.Error(),
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, &DriverError{
			Op:  "initDatabasePool",
			Err: "failed to create database pool: " + err.Error(),
		}
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, &DriverError{
			Op:  "initDatabasePool",
			Err: "failed to ping database: " + err.Error(),
		}
	}

	logger.Logger.Info("Database connected successfully")
	return pool, nil
}

// Close closes the database connection pool
func (d *DatabaseDriver) Close() {
	if d.pool != nil {
		d.pool.Close()
	}
}

func (d *DatabaseDriver) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*ArticleWithTags, *time.Time, string, error) {
	var articles []*ArticleWithTags
	var finalCreatedAt *time.Time
	var finalID string

	var query string
	var args []interface{}

	if lastCreatedAt == nil || lastCreatedAt.IsZero() {
		// First query - no cursor constraint
		query = `
			SELECT a.id, a.title, a.content, a.created_at,
				   COALESCE(
					   array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),
					   '{}'
				   ) as tag_names
			FROM articles a
			LEFT JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN tags t ON at.tag_id = t.id
			GROUP BY a.id, a.title, a.content, a.created_at
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $1
		`
		args = []interface{}{limit}
	} else {
		// Subsequent queries - use efficient keyset pagination
		query = `
			SELECT a.id, a.title, a.content, a.created_at,
				   COALESCE(
					   array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),
					   '{}'
				   ) as tag_names
			FROM articles a
			LEFT JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN tags t ON at.tag_id = t.id
			WHERE (a.created_at, a.id) < ($1, $2)
			GROUP BY a.id, a.title, a.content, a.created_at
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $3
		`
		args = []interface{}{*lastCreatedAt, lastID, limit}
	}

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, "", err
	}
	defer rows.Close()

	for rows.Next() {
		var article ArticleWithTags
		var tagNames []string

		err = rows.Scan(&article.ID, &article.Title, &article.Content, &article.CreatedAt, &tagNames)
		if err != nil {
			return nil, nil, "", err
		}

		// Convert tag names to Tag structs for consistency
		var tags []TagModel
		for _, tagName := range tagNames {
			if tagName != "" {
				tags = append(tags, TagModel{Name: tagName})
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
func (d *DatabaseDriver) GetArticlesWithTagsCount(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM articles`

	var count int
	err := d.pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (d *DatabaseDriver) parseTagsJSON(tagsJSON []byte) ([]TagModel, error) {
	type tagData struct {
		Name string `json:"name"`
	}

	var tags []tagData
	if err := json.Unmarshal(tagsJSON, &tags); err != nil {
		return nil, err
	}

	result := make([]TagModel, len(tags))
	for i, tag := range tags {
		result[i] = TagModel{Name: tag.Name}
	}

	return result, nil
}
