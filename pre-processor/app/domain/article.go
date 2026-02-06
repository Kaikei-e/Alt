package domain

import (
	"time"
)

// Article represents an article entity.
type Article struct {
	CreatedAt   time.Time `db:"created_at"`
	ID          string    `db:"id"`
	Title       string    `db:"title"`
	Content     string    `db:"content"`
	URL         string    `db:"url"`
	FeedID      string    `db:"feed_id"`
	UserID      string    `db:"user_id"`
	PublishedAt time.Time `db:"published_at"`
	InoreaderID string    `db:"inoreader_id"` // Transient field
	FeedURL     string    `db:"-"`            // Transient field for sync
}

// ArticleSummary represents a summary of an article.
type ArticleSummary struct {
	CreatedAt       time.Time `db:"created_at"`
	ID              string    `db:"id"`
	ArticleID       string    `db:"article_id"`
	UserID          string    `db:"user_id"`
	ArticleTitle    string    `db:"article_title"`
	SummaryJapanese string    `db:"summary_japanese"`
}

// ArticleWithSummary represents an article with its summary for quality checking.
type ArticleWithSummary struct {
	CreatedAt       time.Time `json:"created_at"`
	ArticleID       string    `json:"article_id"`
	ArticleTitle    string    `json:"article_title"`
	ArticleContent  string    `json:"article_content"`
	ArticleURL      string    `json:"article_url"`
	SummaryID       string    `json:"summary_id"`
	SummaryJapanese string    `json:"summary_japanese"`
}

// SummarizedContent represents the result from summarization API.
type SummarizedContent struct {
	ArticleID       string `json:"article_id"`
	SummaryJapanese string `json:"summary_japanese"`
}
