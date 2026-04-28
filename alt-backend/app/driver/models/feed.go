package models

import "time"

// Feed mirrors the feeds table; the WebsiteURL field maps to the
// website_url column (renamed from `link` under ADR-000868 — the value
// is the RSS <channel><link> element value, i.e. the website URL of
// the feed source). Distinct from feed_links.url (RSS subscription
// URL) which is referenced via FeedLinkID.
type Feed struct {
	ID          string    `db:"id"`
	Title       string    `db:"title"`
	Description string    `db:"description"`
	WebsiteURL  string    `db:"website_url"`
	PubDate     time.Time `db:"pub_date"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
	// ArticleID is the ID of the corresponding article in the articles table (nullable)
	ArticleID *string `db:"article_id"`
	// IsRead indicates whether this feed has been read by the current user
	IsRead bool `db:"is_read"`
	// FeedLinkID is the ID of the feed_links entry this feed belongs to (nullable)
	FeedLinkID *string `db:"feed_link_id"`
	// OgImageURL is the image URL extracted from RSS (nullable)
	OgImageURL *string `db:"og_image_url"`
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
