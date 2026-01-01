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
)

const upsertArticleQuery = `
	INSERT INTO articles (title, content, url, user_id, feed_id)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (url, user_id) DO UPDATE
	SET title = EXCLUDED.title,
		content = EXCLUDED.content,
		user_id = EXCLUDED.user_id,
		feed_id = COALESCE(EXCLUDED.feed_id, articles.feed_id)
	RETURNING id
`

// SaveArticle stores or updates article content keyed by URL.
func (r *AltDBRepository) SaveArticle(ctx context.Context, url, title, content string) (string, error) {
	if r == nil || r.pool == nil {
		return "", errors.New("database connection not available")
	}

	cleanURL := strings.TrimSpace(url)
	if cleanURL == "" {
		return "", errors.New("article url cannot be empty")
	}

	cleanContent := strings.TrimSpace(content)
	if cleanContent == "" {
		return "", errors.New("article content cannot be empty")
	}

	// Validate minimum content length (already extracted text, should be meaningful)
	const minContentLength = 100
	if len(cleanContent) < minContentLength {
		logger.SafeWarn("article content is very short, may indicate extraction issue",
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
		logger.SafeWarn("article title appears to be a URL, this may indicate a bug", "url", cleanURL, "title", cleanTitle)
	}

	// Extract user_id from context
	userContext, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return "", fmt.Errorf("user context required: %w", err)
	}

	// Get feed_id from URL if possible
	var feedID *uuid.UUID
	feedIDStr, err := r.GetFeedIDByURL(ctx, cleanURL)
	if err != nil {
		// If feed not found, log warning but continue (feed_id will be NULL)
		logger.SafeWarn("feed not found for article URL, article will be saved without feed_id", "url", cleanURL, "error", err)
	} else {
		parsedFeedID, err := uuid.Parse(feedIDStr)
		if err == nil {
			feedID = &parsedFeedID
		}
	}

	// Begin transaction
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				logger.SafeError("failed to rollback transaction", "error", rbErr, "original_error", err)
			}
		}
	}()

	// 1. Upsert Article
	articleID, err := r.UpsertArticleWithTx(ctx, tx, cleanTitle, cleanContent, cleanURL, userContext.UserID, feedID)
	if err != nil {
		return "", err
	}

	// 2. Insert Outbox Event
	eventPayload := map[string]interface{}{
		"article_id": articleID.String(),
		"url":        cleanURL,
		"title":      cleanTitle,
		"body":       cleanContent,
		"user_id":    userContext.UserID.String(),
		"updated_at": time.Now().Format(time.RFC3339),
	}
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal outbox payload: %w", err)
	}

	if err := r.SaveOutboxEventWithTx(ctx, tx, "ARTICLE_UPSERT", payloadBytes); err != nil {
		return "", err
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.SafeInfo("article content saved and outbox event created", "url", cleanURL, "article_id", articleID.String(), "user_id", userContext.UserID)
	return articleID.String(), nil
}
