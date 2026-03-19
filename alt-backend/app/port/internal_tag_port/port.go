// Package internal_tag_port defines interfaces for internal tag API operations.
package internal_tag_port

import (
	"context"
	"time"
)

// TagItem represents a single tag with its confidence score.
type TagItem struct {
	Name       string
	Confidence float32
}

// UpsertArticleTagsPort upserts tags for an article.
type UpsertArticleTagsPort interface {
	UpsertArticleTags(ctx context.Context, articleID string, feedID string, tags []TagItem) (upsertedCount int32, err error)
}

// BatchUpsertItem holds data for a single item in a batch upsert.
type BatchUpsertItem struct {
	ArticleID string
	FeedID    string
	Tags      []TagItem
}

// BatchUpsertArticleTagsPort upserts tags for multiple articles.
type BatchUpsertArticleTagsPort interface {
	BatchUpsertArticleTags(ctx context.Context, items []BatchUpsertItem) (totalUpserted int32, err error)
}

// UntaggedArticle represents an article without tags.
type UntaggedArticle struct {
	ID        string
	Title     string
	Content   string
	UserID    string
	FeedID    *string
	CreatedAt time.Time
}

// ListUntaggedArticlesPort returns articles without tags using keyset pagination.
type ListUntaggedArticlesPort interface {
	ListUntaggedArticles(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) (articles []UntaggedArticle, nextCreatedAt *time.Time, nextID string, totalCount int32, err error)
}
