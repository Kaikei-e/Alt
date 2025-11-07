package domain

import (
	"time"

	"github.com/google/uuid"
)

// RecapArticlesQuery captures the filters supported by the recap articles endpoint.
type RecapArticlesQuery struct {
	From     time.Time
	To       time.Time
	Page     int
	PageSize int
	LangHint *string
	Fields   []string
}

// RecapArticle represents the subset of article data that recap-worker needs.
type RecapArticle struct {
	ID          uuid.UUID
	Title       *string
	FullText    string
	SourceURL   *string
	LangHint    *string
	PublishedAt *time.Time
}

// RecapArticlesPage bundles the paginated result set returned from storage.
type RecapArticlesPage struct {
	Total    int
	Page     int
	PageSize int
	HasMore  bool
	Articles []RecapArticle
}
