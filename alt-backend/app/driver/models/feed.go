package models

import "time"

type Feed struct {
	ID          string    `db:"id"`
	Title       string    `db:"title"`
	Description string    `db:"description"`
	Link        string    `db:"link"`
	PubDate     time.Time `db:"pub_date"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type FeedAndArticle struct {
	FeedID       string `db:"feed_id"`
	ArticleID    string `db:"article_id"`
	URL          string `db:"url"`
	FeedTitle    string `db:"feed_title"`
	ArticleTitle string `db:"article_title"`
}

type InoreaderSummary struct {
	ArticleURL  string    `db:"article_url"`
	Title       string    `db:"title"`
	Author      *string   `db:"author"`
	Content     string    `db:"content"`
	ContentType string    `db:"content_type"`
	PublishedAt time.Time `db:"published_at"`
	FetchedAt   time.Time `db:"fetched_at"`
	InoreaderID string    `db:"inoreader_id"`
}
