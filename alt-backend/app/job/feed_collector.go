package job

import (
	"alt/utils/logger"
	"context"
	"net/url"

	rssFeed "github.com/mmcdole/gofeed"
)

func CollectSingleFeed(ctx context.Context, feedURL url.URL) (*rssFeed.Feed, error) {
	fp := rssFeed.NewParser()
	feed, err := fp.ParseURL(feedURL.String())
	if err != nil {
		logger.Logger.Error("Error parsing feed", "error", err)
		return nil, err
	}

	logger.Logger.Info("Feed collected", "feed title", feed.Title)

	return feed, nil

}
