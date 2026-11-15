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
	Language    string    `db:"language"`
	PublishedAt time.Time `db:"published_at"`
	InoreaderID string    `db:"inoreader_id"` // Transient field
	FeedURL     string    `db:"-"`            // Transient field for sync
}

// Placeholder summaries persisted when an article cannot be summarized.
const (
	PlaceholderTooShort            = "本文が短すぎるため要約できませんでした。"
	PlaceholderTooLong             = "本文が長すぎるため要約できませんでした。"
	PlaceholderDeadLetter          = "本文をモデルが処理できなかったため要約できませんでした。"
	PlaceholderLowQualityExhausted = "品質基準を満たす要約を生成できなかったため要約できませんでした。"
)

// IsPlaceholderSummary reports whether summary is one of the known placeholder texts.
func IsPlaceholderSummary(summary string) bool {
	switch summary {
	case PlaceholderTooShort, PlaceholderTooLong, PlaceholderDeadLetter, PlaceholderLowQualityExhausted:
		return true
	default:
		return false
	}
}

// ArticleSummary represents a summary of an article.
type ArticleSummary struct {
	CreatedAt       time.Time `db:"created_at"`
	ID              string    `db:"id"`
	ArticleID       string    `db:"article_id"`
	UserID          string    `db:"user_id"`
	ArticleTitle    string    `db:"article_title"`
	SummaryJapanese string    `db:"summary_japanese"`
	SkipVersioning  bool      `db:"-"`
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
