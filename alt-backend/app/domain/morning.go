package domain

import (
	"time"

	"github.com/google/uuid"
)

// MorningArticleGroup represents a group of similar articles for the morning update.
type MorningArticleGroup struct {
	GroupID   uuid.UUID
	ArticleID uuid.UUID
	IsPrimary bool
	CreatedAt time.Time
	// Article is the joined article data
	Article *Article
}

// MorningUpdate represents the aggregated update for the frontend.
type MorningUpdate struct {
	GroupID        uuid.UUID  `json:"group_id"`
	PrimaryArticle *Article   `json:"primary_article"`
	Duplicates     []*Article `json:"duplicates"`
}
