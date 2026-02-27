package driver

import (
	"context"
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

const (
	dbMaxRetries = 5
	dbRetryDelay = 5 * time.Second
)

// initDatabasePool initializes the database connection pool with retry logic
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
			slog.ErrorContext(ctx, "invalid SSL configuration", "error", err)
			return nil, &DriverError{
				Op:  "initDatabasePool",
				Err: fmt.Sprintf("SSL configuration error: %v", err),
			}
		}

		slog.InfoContext(ctx, "database configuration",
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

	// Retry logic for database connection
	var pool *pgxpool.Pool
	var lastErr error

	for attempt := 1; attempt <= dbMaxRetries; attempt++ {
		pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
		if err != nil {
			lastErr = err
			slog.WarnContext(ctx, "database connection failed, retrying", "attempt", attempt, "max", dbMaxRetries, "err", err)
			if attempt < dbMaxRetries {
				time.Sleep(dbRetryDelay)
			}
			continue
		}

		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			lastErr = err
			slog.WarnContext(ctx, "database ping failed, retrying", "attempt", attempt, "max", dbMaxRetries, "err", err)
			if attempt < dbMaxRetries {
				time.Sleep(dbRetryDelay)
			}
			continue
		}

		// Connection successful
		break
	}

	if pool == nil {
		return nil, &DriverError{
			Op:  "initDatabasePool",
			Err: fmt.Sprintf("failed to connect to database after %d attempts: %v", dbMaxRetries, lastErr),
		}
	}

	// SSL接続状況確認
	conn, err := pool.Acquire(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Could not acquire connection to check SSL status", "error", err)
	} else {
		defer conn.Release()

		var sslUsed bool
		err := conn.QueryRow(ctx, "SELECT ssl_is_used()").Scan(&sslUsed)
		if err != nil {
			slog.WarnContext(ctx, "Could not check SSL status", "error", err)
		} else {
			slog.InfoContext(ctx, "Database connection established",
				"ssl_enabled", sslUsed,
			)
		}
	}

	logger.Logger.InfoContext(ctx, "Database connected successfully")
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
		// First query - no cursor constraint (Phase 1: Backfill)
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
		// Subsequent queries - use efficient keyset pagination (Phase 1: Backfill)
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

// GetArticlesWithTagsForward fetches articles in forward direction (for incremental indexing)
// This is used in Phase 2 to get new articles created after incrementalMark
func (d *DatabaseDriver) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*ArticleWithTags, *time.Time, string, error) {
	var articles []*ArticleWithTags
	var finalCreatedAt *time.Time
	var finalID string

	var query string
	var args []interface{}

	if lastCreatedAt == nil || lastCreatedAt.IsZero() {
		// First forward query - get articles after incrementalMark
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
		// Subsequent forward queries - use efficient keyset pagination
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

// GetDeletedArticles fetches deleted articles for syncing deletions with Meilisearch
func (d *DatabaseDriver) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]*DeletedArticle, *time.Time, error) {
	var deletedArticles []*DeletedArticle
	var finalDeletedAt *time.Time

	var query string
	var args []interface{}

	if lastDeletedAt == nil || lastDeletedAt.IsZero() {
		// First query - get all deleted articles
		query = `
			SELECT id, deleted_at
			FROM articles
			WHERE deleted_at IS NOT NULL
			ORDER BY deleted_at ASC, id ASC
			LIMIT $1
		`
		args = []interface{}{limit}
	} else {
		// Subsequent queries - use cursor pagination
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

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var deletedArticle DeletedArticle
		err = rows.Scan(&deletedArticle.ID, &deletedArticle.DeletedAt)
		if err != nil {
			return nil, nil, err
		}

		deletedArticles = append(deletedArticles, &deletedArticle)
		// Keep track of the last item for cursor
		finalDeletedAt = &deletedArticle.DeletedAt
	}

	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	return deletedArticles, finalDeletedAt, nil
}

// GetLatestCreatedAt gets the latest created_at timestamp from articles table
// This is used to set the incrementalMark at the start of Phase 1
func (d *DatabaseDriver) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	query := `
		SELECT MAX(created_at)
		FROM articles
		WHERE deleted_at IS NULL
	`

	var latestCreatedAt *time.Time
	err := d.pool.QueryRow(ctx, query).Scan(&latestCreatedAt)
	if err != nil {
		return nil, err
	}

	return latestCreatedAt, nil
}

// GetArticleByID retrieves a single article with tags by its ID.
func (d *DatabaseDriver) GetArticleByID(ctx context.Context, articleID string) (*ArticleWithTags, error) {
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
		GROUP BY a.id, a.title, a.content, a.created_at, a.user_id
	`

	var article ArticleWithTags
	var tagNames []string

	err := d.pool.QueryRow(ctx, query, articleID).Scan(
		&article.ID, &article.Title, &article.Content, &article.CreatedAt, &article.UserID, &tagNames,
	)
	if err != nil {
		return nil, err
	}

	// Convert tag names to Tag structs for consistency
	var tags []TagModel
	for _, tagName := range tagNames {
		if tagName != "" {
			tags = append(tags, TagModel{TagName: tagName})
		}
	}
	article.Tags = tags

	return &article, nil
}
