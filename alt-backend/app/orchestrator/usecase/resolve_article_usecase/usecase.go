package resolve_article_usecase

import (
	"context"
	"fmt"

	"alt/domain"
	"alt/orchestrator/port/article_content_extractor_port"
)

// ArticleRepository defines the storage interface needed for article resolution.
type ArticleRepository interface {
	FetchArticleByID(ctx context.Context, articleID string) (*domain.ArticleContent, error)
	FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error)
	SaveArticle(ctx context.Context, url, title, content string) (string, error)
}

// ResolveArticleUsecase orchestrates article resolution via cache, fallback, or extractor.
type ResolveArticleUsecase interface {
	ResolveArticle(ctx context.Context, feedURL, articleID, content, title string) (string, string, string, error)
}

// Usecase implements ResolveArticleUsecase.
type Usecase struct {
	repo      ArticleRepository
	extractor article_content_extractor_port.ArticleContentExtractorPort
}

// New creates a new ResolveArticleUsecase instance.
func New(repo ArticleRepository, extractor article_content_extractor_port.ArticleContentExtractorPort) *Usecase {
	return &Usecase{
		repo:      repo,
		extractor: extractor,
	}
}

// ResolveArticle resolves article ID and content using DB cache, request fallback, or URL scraping.
func (u *Usecase) ResolveArticle(ctx context.Context, feedURL, articleID, content, title string) (string, string, string, error) {
	if articleID != "" {
		article, err := u.repo.FetchArticleByID(ctx, articleID)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to fetch article by ID: %w", err)
		}
		if article != nil && article.Content != "" {
			if title == "" {
				title = article.Title
			}
			return articleID, title, article.Content, nil
		}
		if content != "" {
			return articleID, title, content, nil
		}
		return "", "", "", fmt.Errorf("article not found or content is empty")
	}

	if feedURL == "" {
		return "", "", "", fmt.Errorf("feed_url or article_id is required")
	}

	existingArticle, err := u.repo.FetchArticleByURL(ctx, feedURL)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch article by URL: %w", err)
	}

	if existingArticle != nil {
		resolvedTitle := title
		if resolvedTitle == "" {
			resolvedTitle = existingArticle.Title
		}
		resolvedContent := content
		if resolvedContent == "" {
			resolvedContent = existingArticle.Content
		}
		return existingArticle.ID, resolvedTitle, resolvedContent, nil
	}

	if content != "" {
		if title == "" {
			title = "No Title"
		}
		newArticleID, err := u.repo.SaveArticle(ctx, feedURL, title, content)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to save article: %w", err)
		}
		return newArticleID, title, content, nil
	}

	fetchedContent, fetchedTitle, err := u.extractor.ExtractArticleContent(ctx, feedURL)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch article content: %w", err)
	}

	if title == "" {
		title = fetchedTitle
	}

	newArticleID, err := u.repo.SaveArticle(ctx, feedURL, title, fetchedContent)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to save article: %w", err)
	}

	return newArticleID, title, fetchedContent, nil
}
