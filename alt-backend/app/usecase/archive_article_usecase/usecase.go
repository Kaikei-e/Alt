package archive_article_usecase

import (
	"alt/port/archive_article_port"
	"alt/port/fetch_article_port"
	"alt/utils/html_parser"
	"context"
	"errors"
	"fmt"
	"strings"
)

// ArchiveArticleInput captures the parameters required to persist article content.
type ArchiveArticleInput struct {
	URL   string
	Title string
}

// ArchiveArticleUsecase coordinates fetching article content and storing it for later retrieval.
type ArchiveArticleUsecase struct {
	fetcher fetch_article_port.FetchArticlePort
	saver   archive_article_port.ArchiveArticlePort
}

// NewArchiveArticleUsecase wires dependencies for archiving articles.
func NewArchiveArticleUsecase(fetcher fetch_article_port.FetchArticlePort, saver archive_article_port.ArchiveArticlePort) *ArchiveArticleUsecase {
	return &ArchiveArticleUsecase{fetcher: fetcher, saver: saver}
}

// Execute performs the archive workflow by fetching article contents and persisting them.
func (u *ArchiveArticleUsecase) Execute(ctx context.Context, input ArchiveArticleInput) error {
	if u == nil {
		return errors.New("archive usecase is not initialized")
	}
	if u.fetcher == nil {
		return errors.New("article fetcher dependency is nil")
	}
	if u.saver == nil {
		return errors.New("article saver dependency is nil")
	}

	cleanURL := strings.TrimSpace(input.URL)
	if cleanURL == "" {
		return errors.New("article URL cannot be empty")
	}

	content, err := u.fetcher.FetchArticleContents(ctx, cleanURL)
	if err != nil {
		return fmt.Errorf("fetch article content: %w", err)
	}
	if content == nil || strings.TrimSpace(*content) == "" {
		return errors.New("fetched article content is empty")
	}

	textOnly := html_parser.ExtractArticleText(*content)
	if textOnly == "" {
		return errors.New("extracted article text is empty")
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = cleanURL
	}

	record := archive_article_port.ArticleRecord{
		URL:     cleanURL,
		Title:   title,
		Content: textOnly,
	}

	if err := u.saver.SaveArticle(ctx, record); err != nil {
		return fmt.Errorf("save article content: %w", err)
	}

	return nil
}
