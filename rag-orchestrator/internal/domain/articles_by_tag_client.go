package domain

import "context"

// TagArticle represents an article retrieved by tag.
type TagArticle struct {
	ID      string
	Title   string
	URL     string
	Content string
}

// ArticlesByTagClient fetches articles filtered by tag from alt-backend.
type ArticlesByTagClient interface {
	FetchArticlesByTag(ctx context.Context, tagName string, limit int) ([]TagArticle, error)
}
