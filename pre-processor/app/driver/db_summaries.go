package driver

import (
	"context"
	"fmt"
	"time"

	"log/slog"
	"pre-processor/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ArticleWithSummary represents an article with its summary for quality checking.
type ArticleWithSummary struct {
	ArticleID       string `db:"article_id"`
	ArticleTitle    string `db:"title"`
	Content         string `db:"content"`
	SummaryJapanese string `db:"summary_japanese"`
	SummaryID       string `db:"summary_id"`
}

// CreateArticleSummary creates a new article summary.
func CreateArticleSummary(ctx context.Context, db *pgxpool.Pool, articleSummary *models.ArticleSummary) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := `
		INSERT INTO article_summaries (article_id, article_title, summary_japanese)
		VALUES ($1, $2, $3)
		ON CONFLICT (article_id) DO NOTHING
		RETURNING id, created_at
	`

	slog.Default().Info("Creating article summary", "article_id", articleSummary.ArticleID)

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		slog.Default().Error("Failed to begin transaction", "error", err)
		return err
	}

	err = tx.QueryRow(ctx, query, articleSummary.ArticleID, articleSummary.ArticleTitle, articleSummary.SummaryJapanese).Scan(
		&articleSummary.ID, &articleSummary.CreatedAt,
	)
	if err != nil {
		err = tx.Rollback(ctx)
		if err != nil {
			slog.Default().Error("Failed to rollback transaction", "error", err)
		}
		slog.Default().Error("Failed to create article summary", "error", err)

		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		slog.Default().Error("Failed to commit transaction", "error", err)
		return err
	}

	slog.Default().Info("Article summary created", "summary_id", articleSummary.ID)

	return nil
}

// GetArticleSummaryByArticleID retrieves an article summary by article ID.
func GetArticleSummaryByArticleID(ctx context.Context, db *pgxpool.Pool, articleID string) (*models.ArticleSummary, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, article_id, article_title, summary_japanese, created_at
		FROM article_summaries
		WHERE article_id = $1
	`

	var summary models.ArticleSummary

	err := db.QueryRow(ctx, query, articleID).Scan(
		&summary.ID, &summary.ArticleID, &summary.ArticleTitle,
		&summary.SummaryJapanese, &summary.CreatedAt,
	)
	if err != nil {
		slog.Default().Error("Failed to get article summary", "error", err)
		return nil, err
	}

	return &summary, nil
}

func GetArticlesWithSummaries(ctx context.Context, db *pgxpool.Pool, lastCreatedAt *time.Time, lastID string, limit int) ([]ArticleWithSummary, *time.Time, string, error) {
	if db == nil {
		return nil, nil, "", fmt.Errorf("database connection is nil")
	}

	var articlesWithSummaries []ArticleWithSummary

	var finalCreatedAt *time.Time

	var finalID string

	err := retryDBOperation(ctx, func() error {
		var query string

		var args []interface{}

		if lastCreatedAt == nil || lastCreatedAt.IsZero() {
			// First query - no cursor constraint
			query = `
				SELECT a_s.article_id, a.content, a_s.summary_japanese, a_s.created_at, a_s.id
				FROM   article_summaries a_s
				JOIN   articles a ON a_s.article_id = a.id
				ORDER  BY a_s.created_at DESC, a_s.id DESC
				LIMIT  $1
			`
			args = []interface{}{limit}
		} else {
			// Subsequent queries - use efficient keyset pagination
			query = `
				SELECT a_s.article_id, a.content, a_s.summary_japanese, a_s.created_at, a_s.id
				FROM   article_summaries a_s
				JOIN   articles a ON a_s.article_id = a.id
				WHERE  (a_s.created_at, a_s.id) < ($1, $2)
				ORDER  BY a_s.created_at DESC, a_s.id DESC
				LIMIT  $3
			`
			args = []interface{}{*lastCreatedAt, lastID, limit}
		}

		rows, err := db.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		articlesWithSummaries = nil // Reset slice for retry

		for rows.Next() {
			var articleWithSummary ArticleWithSummary

			var createdAt time.Time

			var id string

			err = rows.Scan(&articleWithSummary.ArticleID, &articleWithSummary.Content, &articleWithSummary.SummaryJapanese, &createdAt, &id)
			if err != nil {
				return err
			}

			// Store the summary ID in the struct
			articleWithSummary.SummaryID = id

			articlesWithSummaries = append(articlesWithSummaries, articleWithSummary)
			// Keep track of the last item for cursor
			finalCreatedAt = &createdAt
			finalID = id
		}

		return rows.Err()
	}, "GetArticlesWithSummaries")

	if err != nil {
		slog.Default().Error("Failed to get articles with summaries", "error", err)
		return nil, nil, "", err
	}

	slog.Default().Info("Got articles with summaries", "count", len(articlesWithSummaries), "limit", limit, "has_cursor", lastCreatedAt != nil)

	return articlesWithSummaries, finalCreatedAt, finalID, nil
}
