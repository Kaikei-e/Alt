package feed_url_link_gateway

import (
	"alt/driver/models"
	"alt/port/feed_url_link_port"
	"context"
	"log/slog"
)

type FeedURLLinkDriver interface {
	GetFeedURLsByArticleIDs(ctx context.Context, articleIDs []string) ([]models.FeedAndArticle, error)
}

type FeedURLLinkGateway struct {
	driver FeedURLLinkDriver
	logger *slog.Logger
}

func NewFeedURLLinkGateway(driver FeedURLLinkDriver) feed_url_link_port.FeedURLLinkPort {
	return &FeedURLLinkGateway{
		driver: driver,
		logger: slog.Default(),
	}
}

func (g *FeedURLLinkGateway) GetFeedURLsByArticleIDs(ctx context.Context, articleIDs []string) ([]models.FeedAndArticle, error) {
	g.logger.Info("getting feed URLs by article IDs",
		"article_count", len(articleIDs))

	feedAndArticles, err := g.driver.GetFeedURLsByArticleIDs(ctx, articleIDs)
	if err != nil {
		g.logger.Error("failed to get feed URLs by article IDs",
			"error", err,
			"article_count", len(articleIDs))
		return []models.FeedAndArticle{}, err
	}

	g.logger.Info("successfully retrieved feed URLs",
		"requested_count", len(articleIDs),
		"found_count", len(feedAndArticles))

	return feedAndArticles, nil
}
