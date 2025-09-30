package archive_article_gateway

import (
	"alt/port/archive_article_port"
	"context"
	"errors"
	"fmt"
	"strings"
)

// ArticleSaver abstracts the persistence layer that stores article bodies.
type ArticleSaver interface {
	SaveArticle(ctx context.Context, url, title, content string) error
}

// ArchiveArticleGateway provides DB-backed storage for article bodies.
type ArchiveArticleGateway struct {
	repo ArticleSaver
}

// NewArchiveArticleGateway constructs a new gateway instance.
func NewArchiveArticleGateway(saver ArticleSaver) *ArchiveArticleGateway {
	return &ArchiveArticleGateway{repo: saver}
}

// SaveArticle persists the article body using the configured repository.
func (g *ArchiveArticleGateway) SaveArticle(ctx context.Context, record archive_article_port.ArticleRecord) error {
	if g == nil || g.repo == nil {
		return errors.New("article repository is not initialized")
	}

	cleanURL := strings.TrimSpace(record.URL)
	if cleanURL == "" {
		return errors.New("article URL cannot be empty")
	}

	if strings.TrimSpace(record.Content) == "" {
		return errors.New("article content cannot be empty")
	}

	cleanTitle := strings.TrimSpace(record.Title)
	if cleanTitle == "" {
		cleanTitle = cleanURL
	}

	if err := g.repo.SaveArticle(ctx, cleanURL, cleanTitle, record.Content); err != nil {
		return fmt.Errorf("save article content: %w", err)
	}

	return nil
}
