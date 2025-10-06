package driver

import "time"

// ArticleWithTags represents an article with its tags from the database
type ArticleWithTags struct {
	ID        string
	Title     string
	Content   string
	Tags      []TagModel
	CreatedAt time.Time
	UserID    string
}

// TagModel represents a tag from the database
type TagModel struct {
	TagName string
}

// SearchDocumentDriver represents a search document in the search engine
type SearchDocumentDriver struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	UserID  string   `json:"user_id"`
}

// DriverError represents an error from the driver layer
type DriverError struct {
	Op  string
	Err string
}

func (e *DriverError) Error() string {
	return e.Op + ": " + e.Err
}
