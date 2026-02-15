package alt_db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ArticleWithSummaryResult represents an article with its summary from the database.
type ArticleWithSummaryResult struct {
	ArticleID       string
	ArticleContent  string
	ArticleURL      string
	SummaryID       string
	SummaryJapanese string
	CreatedAt       time.Time
}

// DeleteArticleSummaryByArticleID deletes an article summary by article ID.
func (r *AltDBRepository) DeleteArticleSummaryByArticleID(ctx context.Context, articleID string) error {
	query := `DELETE FROM article_summaries WHERE article_id = $1`
	_, err := r.pool.Exec(ctx, query, articleID)
	if err != nil {
		return fmt.Errorf("delete article summary: %w", err)
	}
	return nil
}

// CheckArticleSummaryExists checks if an article summary exists for the given article ID.
func (r *AltDBRepository) CheckArticleSummaryExists(ctx context.Context, articleID string) (bool, string, error) {
	query := `SELECT id FROM article_summaries WHERE article_id = $1 LIMIT 1`
	var summaryID string
	err := r.pool.QueryRow(ctx, query, articleID).Scan(&summaryID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, "", nil
		}
		return false, "", fmt.Errorf("check article summary exists: %w", err)
	}
	return true, summaryID, nil
}

// FindArticlesWithSummaries returns articles with summaries for quality checking.
func (r *AltDBRepository) FindArticlesWithSummaries(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]ArticleWithSummaryResult, *time.Time, string, error) {
	var query string
	var args []interface{}

	if lastCreatedAt == nil || lastCreatedAt.IsZero() {
		query = `
			SELECT a_s.article_id, a.content, a.url, a_s.id, a_s.summary_japanese, a_s.created_at
			FROM   article_summaries a_s
			JOIN   articles a ON a_s.article_id = a.id
			ORDER  BY a_s.created_at DESC, a_s.id DESC
			LIMIT  $1
		`
		args = []interface{}{limit}
	} else {
		query = `
			SELECT a_s.article_id, a.content, a.url, a_s.id, a_s.summary_japanese, a_s.created_at
			FROM   article_summaries a_s
			JOIN   articles a ON a_s.article_id = a.id
			WHERE  (a_s.created_at, a_s.id) < ($1, $2)
			ORDER  BY a_s.created_at DESC, a_s.id DESC
			LIMIT  $3
		`
		args = []interface{}{*lastCreatedAt, lastID, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, "", fmt.Errorf("find articles with summaries: %w", err)
	}
	defer rows.Close()

	var results []ArticleWithSummaryResult
	var finalCreatedAt *time.Time
	var finalID string

	for rows.Next() {
		var item ArticleWithSummaryResult
		err = rows.Scan(&item.ArticleID, &item.ArticleContent, &item.ArticleURL, &item.SummaryID, &item.SummaryJapanese, &item.CreatedAt)
		if err != nil {
			return nil, nil, "", fmt.Errorf("scan article with summary: %w", err)
		}
		results = append(results, item)
		finalCreatedAt = &item.CreatedAt
		finalID = item.SummaryID
	}

	if err = rows.Err(); err != nil {
		return nil, nil, "", fmt.Errorf("iterate articles with summaries: %w", err)
	}

	return results, finalCreatedAt, finalID, nil
}
