package feed_status_port

import (
	"context"
	"net/url"

	"github.com/google/uuid"
)

type UpdateFeedStatusPort interface {
	UpdateFeedStatus(ctx context.Context, feedURL url.URL, userID uuid.UUID) error
}
