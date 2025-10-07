package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"search-indexer/config"
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
		// Create database config with SSL support
		dbConfig := config.NewDatabaseConfigFromEnv()
		dbConfig.User = os.Getenv("SEARCH_INDEXER_DB_USER")
		dbConfig.Password = os.Getenv("SEARCH_INDEXER_DB_PASSWORD")

		// Validate required parameters
		if dbConfig.Host == "" || dbConfig.Port == "" || dbConfig.Name == "" || dbConfig.User == "" || dbConfig.Password == "" {
			return nil, &DriverError{
				Op:  "initDatabasePool",
				Err: "database connection parameters are not set. Required: DB_HOST, DB_PORT, DB_NAME, SEARCH_INDEXER_DB_USER, SEARCH_INDEXER_DB_PASSWORD",
			}
		}

		// SSL設定の検証
		if err := dbConfig.ValidateSSLConfig(); err != nil {
			slog.Error("Invalid SSL configuration", "error", err)
			return nil, &DriverError{
				Op:  "initDatabasePool",
				Err: fmt.Sprintf("SSL configuration error: %v", err),
			}
		}

		slog.Info("Database configuration",
			"host", dbConfig.Host,
			"database", dbConfig.Name,
			"sslmode", dbConfig.SSL.Mode,
		)

		dbURL = dbConfig.BuildPostgresURL()
	}

	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, &DriverError{
			Op:  "initDatabasePool",
			Err: "failed to parse database URL: " + err.Error(),
		}
	}

	// 接続プール設定
	poolConfig.MaxConns = 10
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = time.Minute * 30

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
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

	// SSL接続状況確認
	conn, err := pool.Acquire(ctx)
	if err != nil {
		slog.Warn("Could not acquire connection to check SSL status", "error", err)
	} else {
		defer conn.Release()

		var sslUsed bool
		err := conn.QueryRow(ctx, "SELECT ssl_is_used()").Scan(&sslUsed)
		if err != nil {
			slog.Warn("Could not check SSL status", "error", err)
		} else {
			slog.Info("Database connection established",
				"ssl_enabled", sslUsed,
			)
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
			SELECT a.id, a.title, a.content, a.created_at, a.user_id,
				   COALESCE(
					   array_agg(t.tag_name ORDER BY t.tag_name) FILTER (WHERE t.tag_name IS NOT NULL),
					   '{}'
				   ) as tag_names
			FROM articles a
			LEFT JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN feed_tags t ON at.feed_tag_id = t.id
			GROUP BY a.id, a.title, a.content, a.created_at, a.user_id
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $1
		`
		args = []interface{}{limit}
	} else {
		// Subsequent queries - use efficient keyset pagination
		query = `
			SELECT a.id, a.title, a.content, a.created_at, a.user_id,
				   COALESCE(
					   array_agg(t.tag_name ORDER BY t.tag_name) FILTER (WHERE t.tag_name IS NOT NULL),
					   '{}'
				   ) as tag_names
			FROM articles a
			LEFT JOIN article_tags at ON a.id = at.article_id
			LEFT JOIN feed_tags t ON at.feed_tag_id = t.id
			WHERE (a.created_at, a.id) < ($1, $2)
			GROUP BY a.id, a.title, a.content, a.created_at, a.user_id
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

		err = rows.Scan(&article.ID, &article.Title, &article.Content, &article.CreatedAt, &article.UserID, &tagNames)
		if err != nil {
			return nil, nil, "", err
		}

		// Convert tag names to Tag structs for consistency
		var tags []TagModel
		for _, tagName := range tagNames {
			if tagName != "" {
				tags = append(tags, TagModel{TagName: tagName})
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
		result[i] = TagModel{TagName: tag.Name}
	}

	return result, nil
}
