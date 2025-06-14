package driver

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"pre-processor/logger"
	"pre-processor/models"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// retryDBOperation retries database operations that fail with "conn busy" errors
func retryDBOperation(ctx context.Context, operation func() error, operationName string) error {
	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		// Check if this is a conn busy error
		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<attempt) // Exponential backoff
			logger.Logger.Warn("Database connection busy, retrying",
				"operation", operationName,
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"retry_delay", delay,
				"error", err)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		// If it's not a conn busy error or we've exhausted retries, return the error
		return err
	}

	return fmt.Errorf("operation %s failed after %d retries", operationName, maxRetries)
}

func Init(ctx context.Context) (*pgxpool.Pool, error) {
	// Build connection string
	connString := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable pool_max_conns=20 pool_min_conns=5 pool_max_conn_lifetime=1h pool_max_conn_idle_time=30m",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("PRE_PROCESSOR_DB_USER"),
		os.Getenv("PRE_PROCESSOR_DB_PASSWORD"),
		os.Getenv("DB_NAME"))

	// Parse the connection string to create pool config
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		logger.Logger.Error("Failed to parse database config", "error", err)
		return nil, err
	}

	// Additional pool configuration
	config.MaxConns = 20
	config.MinConns = 5
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	// Create the pool with the configuration
	dbPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		logger.Logger.Error("Failed to connect to database", "error", err)
		return nil, err
	}

	// Test the connection
	err = dbPool.Ping(ctx)
	if err != nil {
		logger.Logger.Error("Failed to ping database", "error", err)
		dbPool.Close()
		return nil, err
	}

	logger.Logger.Info("Connected to database pool", "max_conns", config.MaxConns, "min_conns", config.MinConns)
	return dbPool, nil
}

func GetSourceURLs(offset int, ctx context.Context, db *pgxpool.Pool) ([]url.URL, error) {
	var urls []url.URL

	err := retryDBOperation(ctx, func() error {
		query := `
		SELECT f.link
		FROM feeds f
		LEFT JOIN articles a ON f.link = a.url
		WHERE a.url IS NULL
		AND f.link NOT LIKE '%.mp3'
		ORDER BY f.created_at DESC
		LIMIT 40 OFFSET $1
		`

		rows, err := db.Query(ctx, query, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		urls = nil // Reset urls slice for retry
		for rows.Next() {
			var u string
			err = rows.Scan(&u)
			if err != nil {
				return err
			}

			ul, err := convertToURL(u)
			if err != nil {
				logger.Logger.Error("Failed to convert URL", "error", err)
				continue // Skip invalid URLs but don't fail the whole operation
			}

			urls = append(urls, ul)
		}

		return rows.Err()
	}, "GetSourceURLs")

	if err != nil {
		logger.Logger.Error("Failed to get source URLs", "error", err)
		return nil, err
	}

	// Add diagnostic logging when no URLs found
	if len(urls) == 0 {
		// Check total feeds and processed feeds for debugging
		var totalFeeds, processedFeeds int
		db.QueryRow(ctx, "SELECT COUNT(*) FROM feeds").Scan(&totalFeeds)
		db.QueryRow(ctx, "SELECT COUNT(DISTINCT a.url) FROM articles a INNER JOIN feeds f ON a.url = f.link").Scan(&processedFeeds)

		logger.Logger.Info("No URLs found for processing",
			"offset", offset,
			"total_feeds", totalFeeds,
			"processed_feeds", processedFeeds,
			"remaining_feeds", totalFeeds-processedFeeds)
	}

	logger.Logger.Info("Got source URLs", "count", len(urls), "offset", offset)
	return urls, nil
}

func CheckArticleExists(ctx context.Context, db *pgxpool.Pool, urls []url.URL) (bool, error) {
	if len(urls) == 0 {
		return false, nil
	}

	// Convert url.URL slice to string slice
	urlStrings := make([]string, len(urls))
	for i, u := range urls {
		urlStrings[i] = u.String()
	}

	query := `
		SELECT COUNT(*) FROM articles WHERE url = ANY($1)
	`

	var count int
	err := db.QueryRow(ctx, query, urlStrings).Scan(&count)
	if err != nil {
		return false, err
	}

	return count == len(urls), nil
}

func CreateArticle(ctx context.Context, db *pgxpool.Pool, article *models.Article) error {
	query := `
		INSERT INTO articles (title, content, url)
		VALUES ($1, $2, $3)
		ON CONFLICT (url) DO NOTHING
	`

	logger.Logger.Info("Creating article", "article link", article.URL)
	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.Logger.Error("Failed to begin transaction", "error", err)
		return err
	}

	_, err = tx.Exec(ctx, query, article.Title, article.Content, article.URL)
	if err != nil {
		tx.Rollback(ctx)
		logger.Logger.Error("Failed to create article", "error", err)
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.Error("Failed to commit transaction", "error", err)
		return err
	}

	logger.Logger.Info("Article created", "article", article.Title)

	return nil
}

func CreateArticleSummary(ctx context.Context, db *pgxpool.Pool, articleSummary *models.ArticleSummary) error {
	query := `
		INSERT INTO article_summaries (article_id, article_title, summary_japanese)
		VALUES ($1, $2, $3)
		ON CONFLICT (article_id) DO NOTHING
		RETURNING id, created_at
	`

	logger.Logger.Info("Creating article summary", "article_id", articleSummary.ArticleID)

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.Logger.Error("Failed to begin transaction", "error", err)
		return err
	}

	err = tx.QueryRow(ctx, query, articleSummary.ArticleID, articleSummary.ArticleTitle, articleSummary.SummaryJapanese).Scan(
		&articleSummary.ID, &articleSummary.CreatedAt,
	)
	if err != nil {
		tx.Rollback(ctx)
		logger.Logger.Error("Failed to create article summary", "error", err)
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.Error("Failed to commit transaction", "error", err)
		return err
	}

	logger.Logger.Info("Article summary created", "summary_id", articleSummary.ID)
	return nil
}

func CheckArticleSummarizationCompleted(ctx context.Context, db *pgxpool.Pool, offset int, limit int) (bool, error) {
	query := `
		SELECT COUNT(*)
		FROM articles a
		WHERE a.id NOT IN (SELECT article_id FROM article_summaries)
		LIMIT $1 OFFSET $2
	`

	var count int
	err := db.QueryRow(ctx, query, limit, offset).Scan(&count)
	if err != nil {
		return false, err
	}

	logger.Logger.Info("Checking article summarization completed", "unsummarized_count", count, "offset", offset, "limit", limit)

	return count == 0, nil
}

func GetArticleSummaryByArticleID(ctx context.Context, db *pgxpool.Pool, articleID string) (*models.ArticleSummary, error) {
	query := `
		SELECT id, article_id, summary, summary_japanese, created_at, updated_at
		FROM article_summaries
		WHERE article_id = $1
	`

	var summary models.ArticleSummary
	err := db.QueryRow(ctx, query, articleID).Scan(
		&summary.ID, &summary.ArticleID, &summary.ArticleTitle,
		&summary.SummaryJapanese, &summary.CreatedAt,
	)
	if err != nil {
		logger.Logger.Error("Failed to get article summary", "error", err)
		return nil, err
	}

	return &summary, nil
}

func GetArticlesForSummarization(ctx context.Context, db *pgxpool.Pool, offset int, limit int) ([]*models.Article, error) {
	var articles []*models.Article

	err := retryDBOperation(ctx, func() error {
		query := `
			SELECT a.id, a.title, a.content, a.url, a.created_at
			FROM articles a
			WHERE a.id NOT IN (SELECT article_id FROM article_summaries)
			ORDER BY a.created_at DESC
			LIMIT $1 OFFSET $2
		`

		rows, err := db.Query(ctx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		articles = nil // Reset articles slice for retry
		for rows.Next() {
			var article models.Article
			err = rows.Scan(&article.ID, &article.Title, &article.Content, &article.URL, &article.CreatedAt)
			if err != nil {
				return err
			}
			articles = append(articles, &article)
		}

		return rows.Err()
	}, "GetArticlesForSummarization")

	if err != nil {
		logger.Logger.Error("Failed to get articles without summary", "error", err)
		return nil, err
	}

	logger.Logger.Info("Got articles without summary", "count", len(articles), "offset", offset, "limit", limit)
	return articles, nil
}

func GetFeedStatistics(ctx context.Context, db *pgxpool.Pool) (totalFeeds int, processedFeeds int, err error) {
	// Get total non-MP3 feeds count
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM feeds WHERE link NOT LIKE '%.mp3'").Scan(&totalFeeds)
	if err != nil {
		logger.Logger.Error("Failed to get total non-MP3 feeds count", "error", err)
		return 0, 0, err
	}

	// Get processed non-MP3 feeds count (feeds that have corresponding articles)
	err = db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT f.link)
		FROM feeds f
		INNER JOIN articles a ON f.link = a.url
		WHERE f.link NOT LIKE '%.mp3'
	`).Scan(&processedFeeds)
	if err != nil {
		logger.Logger.Error("Failed to get processed non-MP3 feeds count", "error", err)
		return 0, 0, err
	}

	logger.Logger.Info("Feed statistics (non-MP3 only)", "total_feeds", totalFeeds, "processed_feeds", processedFeeds, "remaining_feeds", totalFeeds-processedFeeds)
	return totalFeeds, processedFeeds, nil
}

func convertToURL(u string) (url.URL, error) {
	ul, err := url.Parse(u)
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to parse URL: %w", err)
	}

	return *ul, nil
}
