package domain

import "time"

// FeedTag represents a tag associated with a feed's articles
type FeedTag struct {
	ID         string    `json:"id"`
	FeedID     string    `json:"feed_id"`
	TagName    string    `json:"tag_name"`
	Confidence float64   `json:"confidence"`
	TagType    string    `json:"tag_type"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TagUpsert is a generated tag on its way back to storage.
//
// It carries the confidence the generator reported and nothing derived from
// it: the name is the tag, the number is how sure the model was, and what to
// do with a low one is the reader's decision rather than the writer's.
type TagUpsert struct {
	Name       string
	Confidence float32
}
