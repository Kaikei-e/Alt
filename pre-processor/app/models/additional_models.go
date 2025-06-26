package models

import (
	"time"
)

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

// ProcessingCursor represents pagination cursor for efficient pagination.
type ProcessingCursor struct {
	LastCreatedAt *time.Time `json:"last_created_at"`
	LastID        string     `json:"last_id"`
}

// Feed represents an RSS feed entry.
type Feed struct {
	CreatedAt time.Time `db:"created_at"`
	ID        string    `db:"id"`
	Link      string    `db:"link"`
	Title     string    `db:"title"`
}

// ProcessingStatistics represents processing statistics.
type ProcessingStatistics struct {
	TotalFeeds     int `json:"total_feeds"`
	ProcessedFeeds int `json:"processed_feeds"`
	RemainingFeeds int `json:"remaining_feeds"`
}
