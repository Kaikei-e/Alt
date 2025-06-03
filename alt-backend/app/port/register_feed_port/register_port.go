package register_feed_port

import "context"

type RegisterFeedPort interface {
	RegisterRSSFeedLink(ctx context.Context, link string) error
}