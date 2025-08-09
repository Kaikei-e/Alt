// ABOUTME: This file defines domain models for Inoreader article metadata
// ABOUTME: Represents article data structure from stream contents API

package models

import (
	"time"

	"github.com/google/uuid"
)

// Article represents an article metadata from Inoreader
type Article struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	InoreaderID      string     `json:"inoreader_id" db:"inoreader_id"`
	SubscriptionID   uuid.UUID  `json:"subscription_id" db:"subscription_id"`
	ArticleURL       string     `json:"article_url" db:"article_url"`
	Title            string     `json:"title" db:"title"`
	Author           string     `json:"author" db:"author"`
	PublishedAt      *time.Time `json:"published_at" db:"published_at"`
	FetchedAt        time.Time  `json:"fetched_at" db:"fetched_at"`
	Processed        bool       `json:"processed" db:"processed"`
	
	// Internal fields for processing (not stored in database)
	OriginStreamID   string     `json:"-" db:"-"`  // Temporary field for UUID resolution
}

// InoreaderStreamResponse represents the Inoreader API response for stream contents
type InoreaderStreamResponse struct {
	Direction    string             `json:"direction"`
	ID           string             `json:"id"`
	Title        string             `json:"title"`
	Description  string             `json:"description"`
	Self         []InoreaderLink    `json:"self"`
	Updated      int64              `json:"updated"`
	Items        []InoreaderItem    `json:"items"`
	Continuation string             `json:"continuation,omitempty"` // For pagination
}

// InoreaderItem represents a single article item from Inoreader API
type InoreaderItem struct {
	ID        string                `json:"id"`        // Unique article ID
	Title     string                `json:"title"`     // Article title
	Published int64                 `json:"published"` // Unix timestamp
	Updated   int64                 `json:"updated"`   // Unix timestamp
	Author    string                `json:"author"`    // Article author
	Canonical []InoreaderLink       `json:"canonical"` // Article URL
	Origin    InoreaderOrigin       `json:"origin"`    // Source feed information
	Summary   InoreaderSummary      `json:"summary"`   // Article summary/content
	Categories []string             `json:"categories"` // Tags/categories
}

// InoreaderLink represents a link in Inoreader API response
type InoreaderLink struct {
	Href string `json:"href"`
	Type string `json:"type,omitempty"`
}

// InoreaderOrigin represents the source feed information
type InoreaderOrigin struct {
	StreamID string `json:"streamId"` // e.g., "feed/http://example.com/rss"
	Title    string `json:"title"`    // Feed title
	HTMLURL  string `json:"htmlUrl"`  // Website URL
}

// InoreaderSummary represents article summary/content
type InoreaderSummary struct {
	Content   string `json:"content"`
	Direction string `json:"direction"`
}

// NewArticle creates a new article from individual parameters  
func NewArticle(inoreaderID, subscriptionID, articleURL, title, author string, publishedAt time.Time) *Article {
	now := time.Now()

	return &Article{
		ID:             uuid.New(),
		InoreaderID:    inoreaderID,
		SubscriptionID: uuid.MustParse(subscriptionID), // Convert string to UUID
		ArticleURL:     articleURL,
		Title:          title,
		Author:         author,
		PublishedAt:    &publishedAt,
		FetchedAt:      now,
		Processed:      false,
	}
}

// NewArticleFromAPI creates a new article from Inoreader API data
func NewArticleFromAPI(inoreaderItem InoreaderItem, subscriptionID uuid.UUID) *Article {
	now := time.Now()
	
	// Extract article URL from canonical links
	articleURL := ""
	if len(inoreaderItem.Canonical) > 0 {
		articleURL = inoreaderItem.Canonical[0].Href
	}

	// Convert Unix timestamp to time.Time
	var publishedAt *time.Time
	if inoreaderItem.Published > 0 {
		published := time.Unix(inoreaderItem.Published, 0)
		publishedAt = &published
	}

	return &Article{
		ID:             uuid.New(),
		InoreaderID:    inoreaderItem.ID,
		SubscriptionID: subscriptionID,
		ArticleURL:     articleURL,
		Title:          inoreaderItem.Title,
		Author:         inoreaderItem.Author,
		PublishedAt:    publishedAt,
		FetchedAt:      now,
		Processed:      false,
	}
}

// NewUUID creates a new UUID
func NewUUID() uuid.UUID {
	return uuid.New()
}

// Now returns the current time
func Now() time.Time {
	return time.Now()
}

// TimeFromUnix converts Unix timestamp to time.Time
func TimeFromUnix(timestamp int64) time.Time {
	return time.Unix(timestamp, 0)
}