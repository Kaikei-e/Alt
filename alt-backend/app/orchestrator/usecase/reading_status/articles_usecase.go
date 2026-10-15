package reading_status

import (
	"alt/domain"
	"alt/orchestrator/port/article_status_port"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/google/uuid"
)

// ArticleByURLLookupPort looks up an archived article by URL.
type ArticleByURLLookupPort interface {
	FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error)
}

// HomeDismissFunc dismisses an item in Knowledge Home when its feed is marked read.
type HomeDismissFunc func(ctx context.Context, userID, tenantID uuid.UUID, itemKey string) error

type ArticlesReadingStatusUsecase struct {
	updateArticleStatusGateway article_status_port.UpdateArticleStatusPort
	articleLookup              ArticleByURLLookupPort
	dismissFn                  HomeDismissFunc
}

func NewArticlesReadingStatusUsecase(updateArticleStatusGateway article_status_port.UpdateArticleStatusPort) *ArticlesReadingStatusUsecase {
	return &ArticlesReadingStatusUsecase{updateArticleStatusGateway: updateArticleStatusGateway}
}

func (u *ArticlesReadingStatusUsecase) WithHomeDismiss(articleLookup ArticleByURLLookupPort, dismissFn HomeDismissFunc) *ArticlesReadingStatusUsecase {
	u.articleLookup = articleLookup
	u.dismissFn = dismissFn
	return u
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
	if err := u.updateArticleStatusGateway.MarkArticleAsRead(ctx, articleURL, user.UserID); err != nil {
		return err
	}
	syncHomeDismiss(ctx, u.articleLookup, u.dismissFn, user, articleURL.String())
	return nil
}

func syncHomeDismiss(ctx context.Context, lookup ArticleByURLLookupPort, dismissFn HomeDismissFunc, user *domain.UserContext, rawURL string) {
	if lookup == nil || dismissFn == nil {
		panic("reading_status: home-dismiss hook is not wired (use WithHomeDismiss)")
	}
	if rawURL == "" {
		return
	}
	article, err := lookup.FetchArticleByURL(ctx, rawURL)
	if err != nil {
		if !errors.Is(err, domain.ErrArticleNotFound) {
			logger.Logger.WarnContext(ctx, "failed to look up article by URL for home dismiss sync",
				"error", err)
		}
		return
	}
	if article == nil || article.ID == "" {
		return
	}
	if err := dismissFn(ctx, user.UserID, user.TenantID, "article:"+article.ID); err != nil {
		logger.Logger.WarnContext(ctx, "failed to sync home dismiss from reading status",
			"error", err, "article_id", article.ID)
	}
}
