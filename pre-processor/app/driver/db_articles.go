package driver

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"pre-processor/domain"

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
func CreateArticle(ctx context.Context, db *pgxpool.Pool, article *domain.Article) error {
	// Validate required UUID fields first (before nil db check to provide specific error messages)
	if article.FeedID == "" {
		return fmt.Errorf("article FeedID is required")
	}
	if article.UserID == "" {
		return fmt.Errorf("article UserID is required")
	}

	if db == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := `
		INSERT INTO articles (title, content, url, feed_id, user_id)
		VALUES ($1, $2, $3, $4, $5)
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

	_, err = tx.Exec(ctx, query, article.Title, article.Content, article.URL, article.FeedID, article.UserID)
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
func GetArticlesForSummarization(ctx context.Context, db *pgxpool.Pool, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	if db == nil {
		return nil, nil, "", fmt.Errorf("database connection is nil")
	}

	var articles []*domain.Article

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
			var article domain.Article

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
func GetArticleByID(ctx context.Context, db *pgxpool.Pool, articleID string) (*domain.Article, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, title, content, url, created_at, user_id
		FROM articles
		WHERE id = $1
	`

	var article domain.Article
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
func GetInoreaderArticles(ctx context.Context, db *pgxpool.Pool, since time.Time) ([]*domain.Article, error) {
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

	var articles []*domain.Article
	for rows.Next() {
		var a domain.Article
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
		a.FeedURL = feedURL     // Set FeedURL for later FeedID resolution

		articles = append(articles, &a)
	}

	return articles, nil
}

// GetInoreaderArticlesForEmptyFeeds fetches inoreader articles whose corresponding
// core feed has zero articles. Feed ID is resolved via feed_links JOIN.
// NOTE: This query requires all tables (inoreader_*, feed_links, feeds, articles)
// to be in the same database. For split-DB deployments, use GetInoreaderArticlesForBackfill instead.
func GetInoreaderArticlesForEmptyFeeds(ctx context.Context, db *pgxpool.Pool, limit int) ([]*domain.Article, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT
			ia.id,
			ia.article_url,
			ia.title,
			ia.content,
			ia.published_at,
			ia.fetched_at,
			f.id AS feed_id
		FROM inoreader_articles ia
		INNER JOIN inoreader_subscriptions isub ON ia.subscription_id = isub.id
		INNER JOIN feed_links fl ON isub.feed_url = fl.url
		INNER JOIN feeds f ON f.feed_link_id = fl.id
		WHERE NOT EXISTS (
			SELECT 1 FROM articles a
			WHERE a.feed_id = f.id AND a.deleted_at IS NULL
		)
		AND ia.content IS NOT NULL
		AND ia.content_length > 0
		ORDER BY ia.fetched_at ASC
		LIMIT $1
	`

	rows, err := db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query inoreader articles for empty feeds: %w", err)
	}
	defer rows.Close()

	var articles []*domain.Article
	for rows.Next() {
		var a domain.Article
		var publishedAt time.Time
		var fetchedAt time.Time

		err := rows.Scan(
			&a.InoreaderID,
			&a.URL,
			&a.Title,
			&a.Content,
			&publishedAt,
			&fetchedAt,
			&a.FeedID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan inoreader article for backfill: %w", err)
		}

		a.PublishedAt = publishedAt
		a.CreatedAt = fetchedAt

		articles = append(articles, &a)
	}

	return articles, nil
}

// GetInoreaderArticlesForBackfill fetches inoreader articles with their feed URLs
// for cross-DB backfill scenarios. Only queries inoreader_* tables (pre-processor-db).
// FeedID resolution is deferred to the caller (via backend API).
// Uses fetchedAfter as a cursor to avoid re-processing the same articles.
func GetInoreaderArticlesForBackfill(ctx context.Context, db *pgxpool.Pool, fetchedAfter time.Time, limit int) ([]*domain.Article, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT
			ia.id,
			ia.article_url,
			ia.title,
			ia.content,
			ia.published_at,
			COALESCE(isub.feed_url, '') AS feed_url,
			ia.fetched_at
		FROM inoreader_articles ia
		INNER JOIN inoreader_subscriptions isub ON ia.subscription_id = isub.id
		WHERE ia.content IS NOT NULL
		AND ia.content_length > 0
		AND ia.fetched_at > $1
		ORDER BY ia.fetched_at ASC
		LIMIT $2
	`

	rows, err := db.Query(ctx, query, fetchedAfter, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query inoreader articles for backfill: %w", err)
	}
	defer rows.Close()

	var articles []*domain.Article
	for rows.Next() {
		var a domain.Article
		var publishedAt time.Time
		var fetchedAt time.Time
		var feedURL string

		err := rows.Scan(
			&a.InoreaderID,
			&a.URL,
			&a.Title,
			&a.Content,
			&publishedAt,
			&feedURL,
			&fetchedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan inoreader article for backfill: %w", err)
		}

		a.PublishedAt = publishedAt
		a.CreatedAt = fetchedAt
		a.FeedURL = feedURL

		articles = append(articles, &a)
	}

	return articles, nil
}

// InsertArticlesBatchNoConflict batch inserts articles, skipping any that already exist (ON CONFLICT DO NOTHING).
// Used by backfill to avoid overwriting full-text articles with Inoreader RSS summaries.
func InsertArticlesBatchNoConflict(ctx context.Context, db *pgxpool.Pool, articles []*domain.Article) (err error) {
	if len(articles) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO articles (title, content, url, feed_id, user_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (url) DO NOTHING
	`

	for _, a := range articles {
		if a.UserID == "" || a.FeedID == "" {
			continue
		}
		batch.Queue(query, a.Title, a.Content, a.URL, a.FeedID, a.UserID, a.CreatedAt)
	}

	if batch.Len() == 0 {
		return nil
	}

	if db == nil {
		return fmt.Errorf("database connection is nil")
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
			return fmt.Errorf("failed to execute batch insert: %w", execErr)
		}
	}

	return nil
}

// UpsertArticlesBatch batches upsert articles
func UpsertArticlesBatch(ctx context.Context, db *pgxpool.Pool, articles []*domain.Article) (err error) {
	if len(articles) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO articles (title, content, url, feed_id, user_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (url) DO UPDATE SET
			title = EXCLUDED.title,
			content = EXCLUDED.content
	`

	for _, a := range articles {
		// Validations: skip articles with empty UserID or FeedID
		if a.UserID == "" || a.FeedID == "" {
			continue
		}
		batch.Queue(query, a.Title, a.Content, a.URL, a.FeedID, a.UserID, a.CreatedAt)
	}

	// After validation, check if batch is empty
	if batch.Len() == 0 {
		return nil
	}

	if db == nil {
		return fmt.Errorf("database connection is nil")
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
