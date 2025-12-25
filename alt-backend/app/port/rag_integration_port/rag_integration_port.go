package rag_integration_port

import (
	"context"
	"time"
)

type RagContext struct {
	ChunkText       string
	URL             string
	Title           string
	PublishedAt     *time.Time
	Score           float32
	DocumentVersion int64
}

type UpsertArticleInput struct {
	ArticleID   string
	Body        string
	PublishedAt *time.Time
	Title       string
	UpdatedAt   *time.Time
	URL         string
	UserID      string
}

type RagIntegrationPort interface {
	RetrieveContext(ctx context.Context, query string, candidateIDs []string) ([]RagContext, error)
	UpsertArticle(ctx context.Context, input UpsertArticleInput) error
}
