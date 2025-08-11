package domain

import "time"

// InoreaderSummary represents an article with summary content from Inoreader
type InoreaderSummary struct {
	ArticleURL     string
	Title          string
	Author         *string
	Content        string
	ContentType    string
	PublishedAt    time.Time
	FetchedAt      time.Time
	InoreaderID    string
}