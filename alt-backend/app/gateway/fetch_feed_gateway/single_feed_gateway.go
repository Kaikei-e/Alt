package fetch_feed_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mmcdole/gofeed"
)

type FetchSingleFeedGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewFetchSingleFeedGateway(db *pgx.Conn) *FetchSingleFeedGateway {
	return &FetchSingleFeedGateway{
		alt_db: alt_db.NewAltDBRepository(db),
	}
}

func (g *FetchSingleFeedGateway) FetchSingleFeed(ctx context.Context) (*domain.RSSFeed, error) {
	// Get RSS feed URLs from the database
	feedURLs, err := g.alt_db.FetchRSSFeedURLs(ctx)
	if err != nil {
		logger.Logger.Error("Error fetching RSS feed URLs", "error", err)
		return nil, fmt.Errorf("failed to fetch RSS feed URLs: %w", err)
	}

	if len(feedURLs) == 0 {
		logger.Logger.Info("No RSS feed URLs found in database")
		return &domain.RSSFeed{
			Title:       "No feeds available",
			Description: "No RSS feed URLs have been registered",
			Items:       []domain.FeedItem{},
		}, nil
	}

	// Use the first available feed URL
	feedURL := feedURLs[0]
	logger.Logger.Info("Fetching RSS feed", "url", feedURL.String())

	// Parse the RSS feed from the URL
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(feedURL.String())
	if err != nil {
		logger.Logger.Error("Error parsing feed", "error", err)
		return nil, fmt.Errorf("failed to parse RSS feed from %s: %w", feedURL.String(), err)
	}

	// Convert the gofeed.Feed to domain.RSSFeed
	domainFeed := convertGofeedToDomain(feed)

	logger.Logger.Info("Successfully fetched RSS feed", "title", domainFeed.Title, "items", len(domainFeed.Items))

	return domainFeed, nil
}

// convertGofeedToDomain converts a gofeed.Feed to domain.RSSFeed
func convertGofeedToDomain(feed *gofeed.Feed) *domain.RSSFeed {
	domainFeed := &domain.RSSFeed{
		Title:       feed.Title,
		Description: feed.Description,
		Link:        feed.Link,
		FeedLink:    feed.FeedLink,
		Updated:     feed.Updated,
		Language:    feed.Language,
		Generator:   feed.Generator,
		FeedType:    feed.FeedType,
		FeedVersion: feed.FeedVersion,
		Items:       make([]domain.FeedItem, 0, len(feed.Items)),
	}

	// Handle updated time parsing
	if feed.UpdatedParsed != nil {
		domainFeed.UpdatedParsed = *feed.UpdatedParsed
	}

	// Handle feed image
	if feed.Image != nil {
		domainFeed.Image = domain.RSSFeedImage{
			URL:   feed.Image.URL,
			Title: feed.Image.Title,
		}
	}

	// Handle feed links
	if len(feed.Links) > 0 {
		domainFeed.Links = make([]string, len(feed.Links))
		for i, link := range feed.Links {
			domainFeed.Links[i] = link
		}
	}

	// Convert feed items
	for _, item := range feed.Items {
		domainItem := domain.FeedItem{
			Title:       item.Title,
			Description: item.Description,
			Link:        item.Link,
			Published:   item.Published,
			Authors: []domain.Author{
				{
					Name: item.Author.Name,
				},
			},
			Links: item.Links,
			Author: domain.Author{
				Name: item.Author.Name,
			},
		}

		// Handle published time parsing
		if item.PublishedParsed != nil {
			domainItem.PublishedParsed = *item.PublishedParsed
		}

		// Handle item links
		if len(item.Links) > 0 {
			domainItem.Links = make([]string, len(item.Links))
			for i, link := range item.Links {
				domainItem.Links[i] = link
			}
		}

		// Handle authors
		if len(item.Authors) > 0 {
			domainItem.Authors = make([]domain.Author, len(item.Authors))
			for i, author := range item.Authors {
				domainItem.Authors[i] = domain.Author{
					Name: author.Name,
				}
			}
			// Set the first author as the main author
			domainItem.Author = domainItem.Authors[0]
		}

		domainFeed.Items = append(domainFeed.Items, domainItem)
	}

	return domainFeed
}
