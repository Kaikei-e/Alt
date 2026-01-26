package driver

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"pre-processor/models"

	logger "pre-processor/utils/logger"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckArticleExists checks if articles already exist in the database.
func CheckArticleExists(ctx context.Context, db *pgxpool.Pool, urls []url.URL) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("database connection is nil")
	}

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

// CreateArticle creates a new article in the database.
func CreateArticle(ctx context.Context, db *pgxpool.Pool, article *models.Article) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := `
		INSERT INTO articles (title, content, url, feed_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (url) DO UPDATE SET
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			url = EXCLUDED.url,
			feed_id = EXCLUDED.feed_id
	`

	// Validate content (already extracted text, should be meaningful)
	const minContentLength = 100
	if len(strings.TrimSpace(article.Content)) < minContentLength {
		logger.Logger.WarnContext(ctx, "article content is very short, may indicate extraction issue",
			"url", article.URL,
			"content_length", len(article.Content))
		// Still allow saving, but log warning
	}

	logger.Logger.InfoContext(ctx, "Creating article", "article link", article.URL)

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to begin transaction", "error", err)
		return err
	}

	_, err = tx.Exec(ctx, query, article.Title, article.Content, article.URL, article.FeedID)
	if err != nil {
		err = tx.Rollback(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to rollback transaction", "error", err)
		}
		logger.Logger.ErrorContext(ctx, "Failed to create article", "error", err)

		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to commit transaction", "error", err)
		return err
	}

	logger.Logger.InfoContext(ctx, "Article created", "article", article.Title)

	return nil
}

// HasUnsummarizedArticles efficiently checks if there are articles without summaries.
func HasUnsummarizedArticles(ctx context.Context, db *pgxpool.Pool) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT EXISTS (
				SELECT 1
				FROM   articles a
				WHERE  NOT EXISTS (
						SELECT 1
						FROM   article_summaries s
						WHERE  s.article_id = a.id
				)
				ORDER  BY a.id
				LIMIT  1
		)
	`

	var hasUnsummarized bool

	err := db.QueryRow(ctx, query).Scan(&hasUnsummarized)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to check for unsummarized articles", "error", err)
		return false, err
	}

	logger.Logger.InfoContext(ctx, "Checked for unsummarized articles", "has_unsummarized", hasUnsummarized)

	return hasUnsummarized, nil
}

// Returns: articles, lastCreatedAt, lastID, error for cursor tracking.
func GetArticlesForSummarization(ctx context.Context, db *pgxpool.Pool, lastCreatedAt *time.Time, lastID string, limit int) ([]*models.Article, *time.Time, string, error) {
	if db == nil {
		return nil, nil, "", fmt.Errorf("database connection is nil")
	}

	var articles []*models.Article

	var finalCreatedAt *time.Time

	var finalID string

	err := retryDBOperation(ctx, func() error {
		var query string

		var args []interface{}

		if lastCreatedAt == nil || lastCreatedAt.IsZero() {
			// First query - no cursor constraint
			query = `
				SELECT a.id, a.title, a.content, a.url, a.created_at, a.user_id
				FROM   articles a
				WHERE  NOT EXISTS (
						SELECT 1
						FROM article_summaries s
						WHERE s.article_id = a.id
				       )
				ORDER BY a.created_at DESC, a.id DESC
				LIMIT $1
			`
			args = []interface{}{limit}
		} else {
			// Subsequent queries - use efficient keyset pagination
			// :ts と :uuid は前ページ最後のカーソル値
			query = `
				SELECT a.id, a.title, a.content, a.url, a.created_at, a.user_id
				FROM   articles a
				WHERE  (a.created_at, a.id) < ($1, $2)
				  AND  NOT EXISTS (
						SELECT 1
						FROM   article_summaries s
						WHERE  s.article_id = a.id
				       )
				ORDER BY a.created_at DESC, a.id DESC
				LIMIT $3
			`
			args = []interface{}{*lastCreatedAt, lastID, limit}
		}

		rows, err := db.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		articles = nil // Reset articles slice for retry

		for rows.Next() {
			var article models.Article

			err = rows.Scan(&article.ID, &article.Title, &article.Content, &article.URL, &article.CreatedAt, &article.UserID)
			if err != nil {
				return err
			}

			articles = append(articles, &article)
			// Keep track of the last item for cursor
			finalCreatedAt = &article.CreatedAt
			finalID = article.ID
		}

		return rows.Err()
	}, "GetArticlesForSummarization")

	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to get articles for summarization", "error", err)
		return nil, nil, "", err
	}

	logger.Logger.InfoContext(ctx, "Got articles for summarization", "count", len(articles), "limit", limit, "has_cursor", lastCreatedAt != nil)

	return articles, finalCreatedAt, finalID, nil
}

// GetArticleByID fetches an article by its ID.
func GetArticleByID(ctx context.Context, db *pgxpool.Pool, articleID string) (*models.Article, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, title, content, url, created_at, user_id
		FROM articles
		WHERE id = $1
	`

	var article models.Article
	err := db.QueryRow(ctx, query, articleID).Scan(
		&article.ID,
		&article.Title,
		&article.Content,
		&article.URL,
		&article.CreatedAt,
		&article.UserID,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found
		}
		logger.Logger.ErrorContext(ctx, "Failed to get article by ID", "error", err, "article_id", articleID)
		return nil, err
	}

	return &article, nil
}

// GetInoreaderArticles fetches articles from inoreader_articles table
func GetInoreaderArticles(ctx context.Context, db *pgxpool.Pool, since time.Time) ([]*models.Article, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// Fetch articles from inoreader_articles
	// Note: feed_url is retrieved via JOIN with inoreader_subscriptions
	query := `
		SELECT
			a.id,
			a.article_url,
			a.title,
			a.content,
			a.published_at,
			COALESCE(s.feed_url, '') AS feed_url,
			a.fetched_at
		FROM inoreader_articles a
		LEFT JOIN inoreader_subscriptions s ON a.subscription_id = s.id
		WHERE a.fetched_at > $1
		ORDER BY a.fetched_at ASC
	`

	rows, err := db.Query(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query inoreader_articles: %w", err)
	}
	defer rows.Close()

	var articles []*models.Article
	for rows.Next() {
		var a models.Article
		var feedURL string
		var publishedAt time.Time
		var fetchedAt time.Time

		err := rows.Scan(
			&a.InoreaderID, // temporarily store inoreader PK in InoreaderID
			&a.URL,
			&a.Title,
			&a.Content,
			&publishedAt,
			&feedURL,
			&fetchedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan inoreader article: %w", err)
		}

		a.PublishedAt = publishedAt
		a.CreatedAt = fetchedAt // Use fetched_at as CreatedAt for now
		// We will resolve FeedID using feedURL later in logic or here if we want to add complexity.
		// For now, let's attach feedURL to Article temporarily?
		// models.Article doesn't have FeedURL field.
		// I'll assume the caller handles it or we add FeedURL to model.
		// I should add FeedURL to model to make this easier.

		articles = append(articles, &a)
	}

	return articles, nil
}

// UpsertArticlesBatch batches upsert articles
func UpsertArticlesBatch(ctx context.Context, db *pgxpool.Pool, articles []*models.Article) (err error) {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}

	if len(articles) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO articles (title, content, url, feed_id, user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (url) DO UPDATE SET
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			updated_at = NOW()
	`

	for _, a := range articles {
		// Validations
		if a.UserID == "" {
			continue // Skip if no UserID
		}
		batch.Queue(query, a.Title, a.Content, a.URL, a.FeedID, a.UserID, a.CreatedAt)
	}

	br := db.SendBatch(ctx, batch)
	defer func() {
		if cerr := br.Close(); cerr != nil {
			if err != nil {
				err = fmt.Errorf("%w; batch close failed: %v", err, cerr)
			} else {
				err = fmt.Errorf("failed to close batch: %w", cerr)
			}
		}
	}()

	for i := 0; i < batch.Len(); i++ {
		_, execErr := br.Exec()
		if execErr != nil {
			// Log error but continue? Or fail batch?
			// Typically fail batch.
			return fmt.Errorf("failed to execute batch upsert: %w", execErr)
		}
	}

	return nil
}
