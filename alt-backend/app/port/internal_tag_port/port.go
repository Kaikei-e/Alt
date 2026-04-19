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

// ArticleTagEntry represents a single tag with provenance metadata.
// The shape matches the outgoing Connect-RPC response and replaces the
// legacy tag-generator /api/v1/tags/batch JSON payload removed per
// ADR-000241 / ADR-000397.
type ArticleTagEntry struct {
	TagName    string
	Confidence float32
	Source     string
	UpdatedAt  time.Time
}

// ArticleTagsByID groups tags by article id for BatchGetTagsByArticleIDs.
type ArticleTagsByID struct {
	ArticleID string
	Tags      []ArticleTagEntry
}

// BatchGetTagsByArticleIDsPort returns tags for a set of article ids.
// Articles without tags are omitted from the response slice.
type BatchGetTagsByArticleIDsPort interface {
	BatchGetTagsByArticleIDs(ctx context.Context, articleIDs []string) ([]ArticleTagsByID, error)
}
