package register_favorite_feed_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/register_favorite_feed_port"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ArticleByURLLookupPort looks up an archived article by URL.
type ArticleByURLLookupPort interface {
	FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error)
}

// HomeOpenFunc records an open action in Knowledge Home when a feed is favorited.
type HomeOpenFunc func(ctx context.Context, userID, tenantID uuid.UUID, itemKey string) error

type RegisterFavoriteFeedUsecase struct {
	gateway       register_favorite_feed_port.RegisterFavoriteFeedPort
	articleLookup ArticleByURLLookupPort
	openFn        HomeOpenFunc
}

func NewRegisterFavoriteFeedUsecase(g register_favorite_feed_port.RegisterFavoriteFeedPort) *RegisterFavoriteFeedUsecase {
	return &RegisterFavoriteFeedUsecase{gateway: g}
}

func (u *RegisterFavoriteFeedUsecase) WithHomeOpen(articleLookup ArticleByURLLookupPort, openFn HomeOpenFunc) *RegisterFavoriteFeedUsecase {
	u.articleLookup = articleLookup
	u.openFn = openFn
	return u
}

func (u *RegisterFavoriteFeedUsecase) Execute(ctx context.Context, url string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("feed url cannot be empty")
	}
	// The authenticated user is resolved here, not below: past this point
	// the call leaves the process that knows who is logged in.
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return fmt.Errorf("authentication required: %w", err)
	}
	if err := u.gateway.RegisterFavoriteFeed(ctx, url, user.UserID); err != nil {
		return err
	}
	if u.articleLookup == nil || u.openFn == nil {
		panic("register_favorite_feed_usecase: home-open hook is not wired (use WithHomeOpen)")
	}
	article, lookupErr := u.articleLookup.FetchArticleByURL(ctx, url)
	if lookupErr != nil {
		logger.Logger.WarnContext(ctx, "failed to look up article by URL for favorite feed home open sync",
			"error_kind", "article_lookup_failed")
		return nil
	}
	if article != nil && article.ID != "" {
		if openErr := u.openFn(ctx, user.UserID, user.TenantID, "article:"+article.ID); openErr != nil {
			logger.Logger.WarnContext(ctx, "failed to sync home open from favorite feed",
				"error_kind", "open_sync_failed", "article_id", article.ID)
		}
	}
	return nil
}
