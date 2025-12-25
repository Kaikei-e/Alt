package rag_integration_port

import (
	"context"
	"time"
)

type UpsertArticleInput struct {
	ArticleID    string
	Title        string
	Body         string
	URL          string
	PublishedAt  time.Time
	UpdatedAt    *time.Time
	UserID       string
}

type RagIntegrationPort interface {
	UpsertArticle(ctx context.Context, input UpsertArticleInput) error
}
