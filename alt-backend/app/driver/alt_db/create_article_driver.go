package alt_db

import (
	"alt/domain"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CreateArticleParams holds parameters for creating an article via internal API.
type CreateArticleParams struct {
	Title       string
	URL         string
	Content     string
	FeedID      string
	UserID      string
	PublishedAt time.Time
}

// CreateArticleInternal creates a new article and returns its ID.
// This is used by the internal API (service-to-service), not the user-facing API.
// If an existing article has longer content, only metadata is updated (content preserved).
// The returned bool is true when a new row was inserted.
func (r *AltDBRepository) CreateArticleInternal(ctx context.Context, params CreateArticleParams) (string, bool, error) {
	if r.pool == nil {
		return "", false, errors.New("database connection not available")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx)

	// 1. Check existing content length
	existingLen, err := r.getArticleContentLength(ctx, tx, params.URL, params.UserID)

	// 2. Choose query: skip content update if existing is longer
	var query string
	if err == nil && existingLen > len(params.Content) {
		query = `
		INSERT INTO articles (title, content, url, feed_id, user_id, published_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (url, user_id) DO UPDATE SET
			title = EXCLUDED.title,
			feed_id = COALESCE(EXCLUDED.feed_id, articles.feed_id),
			published_at = EXCLUDED.published_at
		RETURNING id, (xmax = 0) AS created
	`
	} else {
		query = `
		INSERT INTO articles (title, content, url, feed_id, user_id, published_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (url, user_id) DO UPDATE SET
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			feed_id = COALESCE(EXCLUDED.feed_id, articles.feed_id),
			published_at = EXCLUDED.published_at
		RETURNING id, (xmax = 0) AS created
	`
	}

	// 3. Execute upsert
	var articleID string
	var created bool
	err = tx.QueryRow(ctx, query,
		params.Title,
		params.Content,
		params.URL,
		params.FeedID,
		params.UserID,
		params.PublishedAt,
	).Scan(&articleID, &created)
	if err != nil {
		return "", false, err
	}

	parsedArticleID, err := uuid.Parse(articleID)
	if err != nil {
		return "", false, err
	}
	parsedUserID, err := uuid.Parse(params.UserID)
	if err != nil {
		return "", false, err
	}

	var publishedAt *time.Time
	if !params.PublishedAt.IsZero() {
		publishedAt = &params.PublishedAt
	}
	var knowledgeEvent domain.KnowledgeEvent
	if created {
		knowledgeEvent, err = buildArticleCreatedKnowledgeEvent(parsedArticleID, parsedUserID, &parsedUserID, params.Title, publishedAt)
	} else {
		knowledgeEvent, err = buildArticleUpdatedKnowledgeEvent(parsedArticleID, parsedUserID, &parsedUserID, params.Title, publishedAt)
	}
	if err != nil {
		return "", false, err
	}
	if err := appendKnowledgeEventWithExec(ctx, tx, knowledgeEvent); err != nil {
		return "", false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}

	return articleID, created, nil
}

// getArticleContentLength returns the content length of an existing article within a transaction.
func (r *AltDBRepository) getArticleContentLength(ctx context.Context, tx pgx.Tx, url, userID string) (int, error) {
	var contentLen int
	err := tx.QueryRow(ctx,
		"SELECT COALESCE(LENGTH(content), 0) FROM articles WHERE url = $1 AND user_id = $2 AND deleted_at IS NULL",
		url, userID,
	).Scan(&contentLen)
	return contentLen, err
}
