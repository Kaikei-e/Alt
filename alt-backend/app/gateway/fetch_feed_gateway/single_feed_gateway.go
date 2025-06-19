package fetch_feed_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mmcdole/gofeed"
)

type SingleFeedGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewSingleFeedGateway(pool *pgxpool.Pool) *SingleFeedGateway {
	return &SingleFeedGateway{
		alt_db: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

func (g *SingleFeedGateway) FetchSingleFeed(ctx context.Context) (*domain.RSSFeed, error) {
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}
	// Get RSS feed URLs from the database
	feedURLs, err := g.alt_db.FetchRSSFeedURLs(ctx)
	if err != nil {
		logger.SafeError("Error fetching RSS feed URLs", "error", err)
		return nil, errors.New("error fetching RSS feed URLs")
	}

	if len(feedURLs) == 0 {
		logger.SafeInfo("No RSS feed URLs found in database")
		return &domain.RSSFeed{
			Title:       "No feeds available",
			Description: "No RSS feed URLs have been registered",
			Items:       []domain.FeedItem{},
		}, nil
	}

	// Use the first available feed URL
	feedURL := feedURLs[0]
	logger.SafeInfo("Fetching RSS feed", "url", feedURL.String())

	// Parse the RSS feed from the URL
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(feedURL.String())
	if err != nil {
		logger.SafeError("Error parsing feed", "error", err)
		return nil, errors.New("error parsing feed")
	}

	// Convert the gofeed.Feed to domain.RSSFeed
	domainFeed := convertGofeedToDomain(feed)

	logger.SafeInfo("Successfully fetched RSS feed", "title", domainFeed.Title, "items", len(domainFeed.Items))

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
			Links:       item.Links,
		}

		// Handle Author with nil check
		if item.Author != nil {
			domainItem.Author = domain.Author{
				Name: item.Author.Name,
			}
			domainItem.Authors = []domain.Author{
				{
					Name: item.Author.Name,
				},
			}
		}

		// Handle published time parsing
		if item.PublishedParsed != nil {
			domainItem.PublishedParsed = *item.PublishedParsed
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
