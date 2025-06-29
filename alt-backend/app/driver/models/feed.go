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
