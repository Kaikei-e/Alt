package driver

import (
	"context"
	"fmt"
	"time"

	"pre-processor/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
