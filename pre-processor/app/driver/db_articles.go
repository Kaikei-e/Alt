package driver

import (
	"context"
	"fmt"
	"net/url"
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

	logger.Logger.Info("Creating article", "article link", article.URL)

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.Logger.Error("Failed to begin transaction", "error", err)
		return err
	}

	_, err = tx.Exec(ctx, query, article.Title, article.Content, article.URL)
	if err != nil {
		err = tx.Rollback(ctx)
		if err != nil {
			logger.Logger.Error("Failed to rollback transaction", "error", err)
		}
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
		logger.Logger.Error("Failed to check for unsummarized articles", "error", err)
		return false, err
	}

	logger.Logger.Info("Checked for unsummarized articles", "has_unsummarized", hasUnsummarized)

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
				SELECT a.id, a.title, a.content, a.url, a.created_at
				FROM   articles a
				WHERE  NOT EXISTS (
						SELECT 1
						FROM   article_summaries s
						WHERE  s.article_id = a.id
				       )
				ORDER BY a.created_at DESC, a.id DESC
				LIMIT $1
			`
			args = []interface{}{limit}
		} else {
			// Subsequent queries - use efficient keyset pagination
			// :ts と :uuid は前ページ最後のカーソル値
			query = `
				SELECT a.id, a.title, a.content, a.url, a.created_at
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

			err = rows.Scan(&article.ID, &article.Title, &article.Content, &article.URL, &article.CreatedAt)
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
		logger.Logger.Error("Failed to get articles for summarization", "error", err)
		return nil, nil, "", err
	}

	logger.Logger.Info("Got articles for summarization", "count", len(articles), "limit", limit, "has_cursor", lastCreatedAt != nil)

	return articles, finalCreatedAt, finalID, nil
}
