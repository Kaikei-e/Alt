package reading_status

import (
	"alt/port/feed_status_port"
	"context"
	"net/url"
)

type FeedsReadingStatusUsecase struct {
	updateFeedStatusGateway feed_status_port.UpdateFeedStatusPort
}

func NewFeedsReadingStatusUsecase(updateFeedStatusGateway feed_status_port.UpdateFeedStatusPort) *FeedsReadingStatusUsecase {
	return &FeedsReadingStatusUsecase{updateFeedStatusGateway: updateFeedStatusGateway}
}

func (u *FeedsReadingStatusUsecase) Execute(ctx context.Context, feedURL url.URL) error {
	return u.updateFeedStatusGateway.UpdateFeedStatus(ctx, feedURL)
}
