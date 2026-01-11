package feed_link_availability_port

import (
	"alt/domain"
	"context"
)

// FeedLinkAvailabilityPort defines operations for managing feed link availability state.
type FeedLinkAvailabilityPort interface {
	// IncrementFeedLinkFailures increments the failure count and returns the updated availability.
	IncrementFeedLinkFailures(ctx context.Context, feedURL, reason string) (*domain.FeedLinkAvailability, error)

	// ResetFeedLinkFailures resets the failure count on successful fetch.
	ResetFeedLinkFailures(ctx context.Context, feedURL string) error

	// DisableFeedLink marks a feed as inactive.
	DisableFeedLink(ctx context.Context, feedURL string) error
}
