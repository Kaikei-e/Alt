package domain

import "time"

type SearchDocument struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Tags        []string  `json:"tags"`
	UserID      string    `json:"user_id"`
	Language    string    `json:"language"`
	Score       float64   `json:"score"`
	PublishedAt time.Time `json:"published_at"`
}

func NewSearchDocument(article *Article) SearchDocument {
	return SearchDocument{
		ID:          article.ID(),
		Title:       article.Title(),
		Content:     article.Content(),
		Tags:        article.Tags(),
		UserID:      article.UserID(),
		Language:    article.Language(),
		PublishedAt: article.PublishedAt(),
	}
}
