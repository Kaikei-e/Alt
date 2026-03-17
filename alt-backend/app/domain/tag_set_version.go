package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// TagSetVersion represents a versioned tag set snapshot.
type TagSetVersion struct {
	TagSetVersionID uuid.UUID       `json:"tag_set_version_id" db:"tag_set_version_id"`
	ArticleID       uuid.UUID       `json:"article_id" db:"article_id"`
	UserID          uuid.UUID       `json:"user_id" db:"user_id"`
	GeneratedAt     time.Time       `json:"generated_at" db:"generated_at"`
	Generator       string          `json:"generator" db:"generator"`
	InputHash       string          `json:"input_hash" db:"input_hash"`
	TagsJSON        json.RawMessage `json:"tags_json" db:"tags_json"`
	SupersededBy    *uuid.UUID      `json:"superseded_by" db:"superseded_by"`
}
