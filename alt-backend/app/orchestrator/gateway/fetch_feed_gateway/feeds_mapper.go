package fetch_feed_gateway

import (
	"alt/domain"
	"alt/orchestrator/driver/models"
	"alt/utils/sanitize"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// mapFeedBasic maps a database feed row to a basic domain.FeedItem without parsing feed ID.
func mapFeedBasic(feed *models.Feed) *domain.FeedItem {
	publishedTime := feed.CreatedAt
	return &domain.FeedItem{
		Title:           feed.Title,
		Description:     sanitize.SanitizeDescription(feed.Description),
		Link:            feed.WebsiteURL,
		Published:       publishedTime.Format(time.RFC3339),
		PublishedParsed: publishedTime,
	}
}

// mapFeedCursor maps a database feed row to a cursor-paginated domain.FeedItem, parsing feed.ID
// and optionally including the IsRead status (only FetchFeedsListCursor includes IsRead).
func mapFeedCursor(feed *models.Feed, includeIsRead bool) (*domain.FeedItem, error) {
	publishedTime := feed.CreatedAt

	feedID, err := parseFeedID(feed.ID)
	if err != nil {
		return nil, err
	}

	feedItem := &domain.FeedItem{
		FeedID:          feedID,
		Title:           feed.Title,
		Description:     sanitize.SanitizeDescription(feed.Description),
		Link:            feed.WebsiteURL,
		Published:       publishedTime.Format(time.RFC3339),
		PublishedParsed: publishedTime,
		OgImageURL:      derefString(feed.OgImageURL),
	}

	if includeIsRead {
		feedItem.IsRead = feed.IsRead
	}

	if feed.ArticleID != nil {
		feedItem.ArticleID = *feed.ArticleID
	}

	return feedItem, nil
}

// parseFeedID turns a row's feeds.id column into the UUID a domain.FeedItem
// carries, so the client can hand it back to ResolveOgImages.
//
// This is the hop the on-demand og:image resolution was missing: every cursor
// query above already selects f.id and every row already carries it, but the
// four mappers dropped it here, leaving FeedItem.FeedID at uuid.Nil and the
// client with nothing to send but articles.id — which the resolver's
// `WHERE f.id = ANY($1::uuid[])` never matches.
//
// A bad value fails the page rather than blanking one row's id. feeds.id is a
// uuid PRIMARY KEY, so an unparseable one does not mean "this feed is slightly
// off"; it means the column being walked is not the column this code thinks it
// is, and a silently-empty feed_id is precisely the indistinguishable-from-
// working failure the field was added to end (CLAUDE.md rule 8).
// feed_page_cache_gateway makes the same call on the same conversion.
func parseFeedID(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse feed id %q: %w", id, err)
	}
	return parsed, nil
}

// derefString safely dereferences a *string, returning "" if nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
