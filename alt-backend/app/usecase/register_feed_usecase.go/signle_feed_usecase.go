package register_feed_usecase

import (
	"alt/port/register_feed_port"
	"alt/utils/logger"
	"context"

	"github.com/mmcdole/gofeed"
)

type RegisterFeedUsecase struct {
	registerFeedGateway register_feed_port.RegisterFeedPort
}

func NewRegisterFeedUsecase(registerFeedGateway register_feed_port.RegisterFeedPort) *RegisterFeedUsecase {
	return &RegisterFeedUsecase{registerFeedGateway: registerFeedGateway}
}

func (r *RegisterFeedUsecase) Execute(ctx context.Context, link string) error {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(link)
	if err != nil {
		logger.Logger.Error("Error parsing feed", "error", err)
		return err
	}

	logger.Logger.Info("Feed parsed", "rss feed link", feed.Link)

	return r.registerFeedGateway.RegisterRSSFeedLink(ctx, feed.Link)
}
