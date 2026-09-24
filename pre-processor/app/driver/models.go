package driver

import "time"

// ArticleWithSummary represents an article with its summary for quality checking.
type ArticleWithSummary struct {
	ArticleID       string `db:"article_id"`
	ArticleTitle    string `db:"title"`
	Content         string `db:"content"`
	SummaryJapanese string `db:"summary_japanese"`
	SummaryID       string `db:"summary_id"`
}

// InoreaderArticleRow represents a row from the inoreader_articles table.
type InoreaderArticleRow struct {
	ID          string
	ArticleURL  string
	Title       string
	Content     string
	PublishedAt time.Time
	FeedURL     string
	FetchedAt   time.Time
}
