package feed_link_availability_port

import (
	"alt/domain"
	"context"
)

// FeedLinkAvailabilityPort is the collector's view of poll health.
//
// Two methods where there used to be three. IncrementFeedLinkFailures and
// DisableFeedLink were separate, and every caller used them as one operation:
// increment, read the returned count, compare it to a threshold, disable. That
// read-modify-write raced with itself — two collector ticks on the same broken
// feed could each read a count one below the threshold and neither disable it
// — and ADR-000954 moved the implementation into another process, which would
// have stretched the window across a network round trip.
//
// RecordFeedLinkFailure closes it: the comparison happens inside the
// provider's transaction, and this interface offers no way to take it apart
// again (capability catalog §4-4).
type FeedLinkAvailabilityPort interface {
	// RecordFeedLinkFailure counts one failure and, in the same transaction,
	// disables the link once the consecutive run reaches the implementation's
	// configured threshold.
	//
	// The bool reports the transition to inactive, not the state. A caller
	// that logged the state would re-raise the same auto-disable alert on
	// every poll of a feed that has been dead for weeks.
	RecordFeedLinkFailure(ctx context.Context, feedURL, reason string) (*domain.FeedLinkAvailability, bool, error)

	// ResetFeedLinkFailures clears the run and re-activates the link after a
	// successful poll.
	ResetFeedLinkFailures(ctx context.Context, feedURL string) error
}
