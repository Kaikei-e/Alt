package fetch_article_usecase

import (
	"alt/port/fetch_article_port"
	"alt/utils/html_parser"
	"context"
	"errors"
	"strings"
)

type ArticleUsecase interface {
	Execute(ctx context.Context, articleURL string) (*string, error)
}

type ArticleUsecaseImpl struct {
	articleFetcher fetch_article_port.FetchArticlePort
}

func NewArticleUsecase(articleFetcher fetch_article_port.FetchArticlePort) ArticleUsecase {
	return &ArticleUsecaseImpl{articleFetcher: articleFetcher}
}

func (u *ArticleUsecaseImpl) Execute(ctx context.Context, articleURL string) (*string, error) {
	// Fetch raw HTML content
	content, err := u.articleFetcher.FetchArticleContents(ctx, articleURL)
	if err != nil {
		return nil, err
	}

	// Check if content is nil or empty
	if content == nil || strings.TrimSpace(*content) == "" {
		return nil, errors.New("fetched article content is empty")
	}

	// Extract text from HTML, removing images, scripts, styles, etc.
	textOnly := html_parser.ExtractArticleText(*content)
	if strings.TrimSpace(textOnly) == "" {
		return nil, errors.New("extracted article text is empty")
	}

	return &textOnly, nil
}
