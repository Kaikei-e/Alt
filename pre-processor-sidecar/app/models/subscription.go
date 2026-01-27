// ABOUTME: This file defines domain models for Inoreader subscription management
// ABOUTME: Represents subscription data structure and related operations

package models

import (
	"time"

	"github.com/google/uuid"
)

// Subscription represents an Inoreader RSS feed subscription
type Subscription struct {
	ID          uuid.UUID `json:"id" db:"id"`
	InoreaderID string    `json:"inoreader_id" db:"inoreader_id"`
	FeedURL     string    `json:"feed_url" db:"feed_url"`
	Title       string    `json:"title" db:"title"`
	Category    string    `json:"category" db:"category"`
	SyncedAt    time.Time `json:"synced_at" db:"synced_at"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// InoreaderSubscriptionResponse represents the Inoreader API response for subscription list
type InoreaderSubscriptionResponse struct {
	Subscriptions []InoreaderSubscription `json:"subscriptions"`
}

// InoreaderSubscription represents a single subscription from Inoreader API
type InoreaderSubscription struct {
	DatabaseID  uuid.UUID           `json:"database_id" db:"id"` // Database primary key UUID
	InoreaderID string              `json:"id"`                  // e.g., "feed/http://example.com/rss" from API
	Title       string              `json:"title"`               // Feed title
	Categories  []InoreaderCategory `json:"categories"`          // Folder/label information
	URL         string              `json:"url"`                 // XML feed URL
	HTMLURL     string              `json:"htmlUrl"`             // Website URL
	IconURL     string              `json:"iconUrl"`             // Favicon URL
	CreatedAt   time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at" db:"updated_at"`
}

// InoreaderCategory represents a category/folder in Inoreader
type InoreaderCategory struct {
	ID    string `json:"id"`    // e.g., "user/1234/label/News"
	Label string `json:"label"` // Display name
}

// NewSubscription creates a new subscription from individual parameters
func NewSubscription(inoreaderID, feedURL, title, category string) *Subscription {
	now := time.Now()

	return &Subscription{
		ID:          uuid.New(),
		InoreaderID: inoreaderID,
		FeedURL:     feedURL,
		Title:       title,
		Category:    category,
		SyncedAt:    now,
		CreatedAt:   now,
	}
}

// NewSubscriptionFromAPI creates a new subscription from Inoreader API data
func NewSubscriptionFromAPI(inoreaderSub InoreaderSubscription) *Subscription {
	now := time.Now()

	// Extract category from categories slice (use first category if multiple)
	category := ""
	if len(inoreaderSub.Categories) > 0 {
		category = inoreaderSub.Categories[0].Label
	}

	return &Subscription{
		ID:          uuid.New(),
		InoreaderID: inoreaderSub.InoreaderID,
		FeedURL:     inoreaderSub.URL,
		Title:       inoreaderSub.Title,
		Category:    category,
		SyncedAt:    now,
		CreatedAt:   now,
	}
}

// UpdateFromInoreader updates subscription data from Inoreader API
func (s *Subscription) UpdateFromInoreader(inoreaderSub InoreaderSubscription) {
	s.Title = inoreaderSub.Title
	s.FeedURL = inoreaderSub.URL

	// Update category (use first category if multiple)
	if len(inoreaderSub.Categories) > 0 {
		s.Category = inoreaderSub.Categories[0].Label
	}

	s.SyncedAt = time.Now()
}
