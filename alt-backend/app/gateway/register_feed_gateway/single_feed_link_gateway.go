package register_feed_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mmcdole/gofeed"
)

type RegisterFeedGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewRegisterFeedLinkGateway(pool *pgxpool.Pool) *RegisterFeedGateway {
	return &RegisterFeedGateway{alt_db: alt_db.NewAltDBRepositoryWithPool(pool)}
}

func (g *RegisterFeedGateway) RegisterRSSFeedLink(ctx context.Context, link string) error {
	if g.alt_db == nil {
		return errors.New("database connection not available")
	}
	// Parse and validate the URL
	parsedURL, err := url.Parse(link)
	if err != nil {
		return errors.New("invalid URL format")
	}

	// Ensure the URL has a scheme
	if parsedURL.Scheme == "" {
		return errors.New("URL must include a scheme (http or https)")
	}

	// Try to fetch and parse the RSS feed to validate it
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(link)
	if err != nil {
		if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "connection refused") {
			return errors.New("could not reach the RSS feed URL")
		}
		return errors.New("invalid RSS feed format")
	}

	if feed.Link == "" {
		logger.SafeWarn("RSS feed link is empty, using the link from the RSS feed", "link", link)
		feed.Link = link
	}

	if feed.FeedLink == "" {
		logger.SafeWarn("RSS feed feed link is empty, using the link from the RSS feed", "link", feed.Link)
		feed.FeedLink = link
	}

	err = g.alt_db.RegisterRSSFeedLink(ctx, feed.FeedLink)
	if err != nil {
		if errors.Is(err, pgx.ErrTxClosed) {
			logger.SafeError("Failed to register RSS feed link", "error", err)
			return errors.New("failed to register RSS feed link")
		}
		logger.SafeError("Error registering RSS feed link", "error", err)
		return errors.New("failed to register RSS feed link")
	}
	logger.SafeInfo("RSS feed link registered", "link", link)

	return nil
}
