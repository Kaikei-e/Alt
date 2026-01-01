package reading_status

import (
	"alt/port/article_status_port"
	"context"
	"net/url"
)

type ArticlesReadingStatusUsecase struct {
	updateArticleStatusGateway article_status_port.UpdateArticleStatusPort
}

func NewArticlesReadingStatusUsecase(updateArticleStatusGateway article_status_port.UpdateArticleStatusPort) *ArticlesReadingStatusUsecase {
	return &ArticlesReadingStatusUsecase{updateArticleStatusGateway: updateArticleStatusGateway}
}

func (u *ArticlesReadingStatusUsecase) Execute(ctx context.Context, articleURL url.URL) error {
	return u.updateArticleStatusGateway.MarkArticleAsRead(ctx, articleURL)
}
