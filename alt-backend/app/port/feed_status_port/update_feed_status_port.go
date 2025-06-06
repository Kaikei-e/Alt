package feed_status_port

import (
	"context"
	"net/url"
)

type UpdateFeedStatusPort interface {
	UpdateFeedStatus(ctx context.Context, feedURL url.URL) error
}
