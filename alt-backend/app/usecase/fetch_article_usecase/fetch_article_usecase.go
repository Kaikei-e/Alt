package fetch_article_usecase

import (
	"alt/port/fetch_article_port"
	"context"
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
	return u.articleFetcher.FetchArticleContents(ctx, articleURL)
}
