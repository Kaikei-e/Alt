package reading_status

import (
	"alt/domain"
	"alt/orchestrator/port/feed_status_port"
	"context"
	"fmt"
	"net/url"
)

type FeedsReadingStatusUsecase struct {
	updateFeedStatusGateway feed_status_port.UpdateFeedStatusPort
	articleLookup           ArticleByURLLookupPort
	dismissFn               HomeDismissFunc
}

func NewFeedsReadingStatusUsecase(updateFeedStatusGateway feed_status_port.UpdateFeedStatusPort) *FeedsReadingStatusUsecase {
	return &FeedsReadingStatusUsecase{updateFeedStatusGateway: updateFeedStatusGateway}
}

func (u *FeedsReadingStatusUsecase) WithHomeDismiss(articleLookup ArticleByURLLookupPort, dismissFn HomeDismissFunc) *FeedsReadingStatusUsecase {
	u.articleLookup = articleLookup
	u.dismissFn = dismissFn
	return u
}

func (u *FeedsReadingStatusUsecase) Execute(ctx context.Context, feedURL url.URL) error {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return fmt.Errorf("authentication required: %w", err)
	}
	if err := u.updateFeedStatusGateway.UpdateFeedStatus(ctx, feedURL, user.UserID); err != nil {
		return err
	}
	syncHomeDismiss(ctx, u.articleLookup, u.dismissFn, user, feedURL.String())
	return nil
}
