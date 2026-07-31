package reading_status

import (
	"alt/domain"
	"alt/orchestrator/port/article_status_port"
	"context"
	"fmt"
	"net/url"
)

type ArticlesReadingStatusUsecase struct {
	updateArticleStatusGateway article_status_port.UpdateArticleStatusPort
}

func NewArticlesReadingStatusUsecase(updateArticleStatusGateway article_status_port.UpdateArticleStatusPort) *ArticlesReadingStatusUsecase {
	return &ArticlesReadingStatusUsecase{updateArticleStatusGateway: updateArticleStatusGateway}
}

// Execute resolves the authenticated reader and marks the article's feed read.
//
// The user lookup lives here rather than in the gateway or the driver, the way
// FeedsReadingStatusUsecase has always done it. Below this point the call
// crosses a process boundary where "the current user" no longer exists.
func (u *ArticlesReadingStatusUsecase) Execute(ctx context.Context, articleURL url.URL) error {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return fmt.Errorf("authentication required: %w", err)
	}
	return u.updateArticleStatusGateway.MarkArticleAsRead(ctx, articleURL, user.UserID)
}
