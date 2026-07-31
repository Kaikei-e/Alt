package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const upsertArticleQuery = `
	INSERT INTO articles (title, content, url, user_id, feed_id)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (url, user_id) DO UPDATE
	SET title = EXCLUDED.title,
		content = EXCLUDED.content,
		user_id = EXCLUDED.user_id,
		feed_id = COALESCE(EXCLUDED.feed_id, articles.feed_id)
	RETURNING id, (xmax = 0) AS created
`

// SaveArticle stores or updates article content keyed by URL, for the user in
// the request context.
//
// It is the in-process entry point. Everything below the user lookup lives in
// SaveArticleForUser, which alt-data-hub calls with the owner as an argument
// because a Connect request carries no user context (ADR-000954 Wave 3,
// catalog §2.B).
func (r *ArticleRepository) SaveArticle(ctx context.Context, url, title, content string) (string, error) {
	if r == nil || r.pool == nil {
		return "", errors.New("database connection not available")
	}

	userContext, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return "", fmt.Errorf("user context required: %w", err)
	}

	articleID, _, err := r.SaveArticleForUser(ctx, url, title, content, userContext.UserID)
	return articleID, err
}

// SaveArticleForUser upserts the article and appends its ARTICLE_UPSERT outbox
// row in one transaction, and reports whether the row was newly inserted.
//
// The two writes share a transaction on purpose: an article rag-orchestrator
// never hears about, or an event for an article that was never written, are
// both unrecoverable by retry because nothing records that the pair was meant
// to be atomic.
func (r *ArticleRepository) SaveArticleForUser(ctx context.Context, url, title, content string, userID uuid.UUID) (string, bool, error) {
	if r == nil || r.pool == nil {
		return "", false, errors.New("database connection not available")
	}

	if userID == uuid.Nil {
		// articles is keyed by (url, user_id). Writing the zero UUID would
		// create a row that no tenant-scoped query can ever read back, so the
		// article would look saved and be invisible.
		return "", false, errors.New("article user id cannot be empty")
	}

	cleanURL := strings.TrimSpace(url)
	if cleanURL == "" {
		return "", false, errors.New("article url cannot be empty")
	}

	cleanContent := strings.TrimSpace(content)
	if cleanContent == "" {
		return "", false, errors.New("article content cannot be empty")
	}

	// Validate minimum content length (already extracted text, should be meaningful)
	const minContentLength = 100
	if len(cleanContent) < minContentLength {
		logger.SafeWarnContext(ctx, "article content is very short, may indicate extraction issue",
			"url", cleanURL,
			"content_length", len(cleanContent))
		// Still allow saving, but log warning
	}

	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = cleanURL
	}

	// Validate that title is not a URL (this would indicate a bug)
	if strings.HasPrefix(cleanTitle, "http://") || strings.HasPrefix(cleanTitle, "https://") {
		logger.SafeWarnContext(ctx, "article title appears to be a URL, this may indicate a bug", "url", cleanURL, "title", cleanTitle)
	}

	// Get feed_id from URL if possible
	var feedID *uuid.UUID
	feedIDStr, err := getFeedIDByArticleURL(ctx, r.pool, cleanURL)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			// A real DB failure (timeout, connection reset, ...) must not be
			// reinterpreted as "no matching feed" — that would silently
			// persist the article with feed_id=NULL even though a working
			// lookup might have resolved a real feed (finding [14]).
			return "", false, fmt.Errorf("resolve feed id for article: %w", err)
		}
		// Genuinely no feed row for this URL: continue, feed_id stays NULL.
		logger.SafeWarnContext(ctx, "feed not found for article URL, article will be saved without feed_id", "url", cleanURL)
	} else {
		parsedFeedID, err := uuid.Parse(feedIDStr)
		if err == nil {
			feedID = &parsedFeedID
		}
	}

	// Begin transaction
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Always rollback - no-op after successful Commit

	// 1. Upsert Article
	articleID, created, err := r.UpsertArticleWithTx(ctx, tx, cleanTitle, cleanContent, cleanURL, userID, feedID)
	if err != nil {
		return "", false, err
	}

	// 2. Insert Outbox Event
	eventPayload := map[string]interface{}{
		"article_id": articleID.String(),
		"url":        cleanURL,
		"title":      cleanTitle,
		"body":       cleanContent,
		"user_id":    userID.String(),
		"updated_at": time.Now().Format(time.RFC3339),
	}
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return "", false, fmt.Errorf("failed to marshal outbox payload: %w", err)
	}

	if err := saveOutboxEventWithTx(ctx, tx, "ARTICLE_UPSERT", payloadBytes); err != nil {
		return "", false, err
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return "", false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.SafeInfoContext(ctx, "article content saved and outbox event created", "url", cleanURL, "article_id", articleID.String(), "user_id", userID)
	return articleID.String(), created, nil
}
