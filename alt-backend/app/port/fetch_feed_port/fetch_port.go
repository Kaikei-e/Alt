package fetch_feed_port

import "alt/domain"

type FetchSingleFeedPort interface {
	FetchSingleFeed() (*domain.RSSFeed, error)
}
