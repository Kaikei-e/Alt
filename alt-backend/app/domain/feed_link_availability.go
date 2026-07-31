package domain

import (
	"time"

	"github.com/google/uuid"
)

// FeedLinkAvailability represents the health state of a feed link.
// It tracks whether the feed is active and records failure information.
type FeedLinkAvailability struct {
	FeedLinkID          uuid.UUID  `json:"feed_link_id"`
	IsActive            bool       `json:"is_active"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	LastFailureReason   *string    `json:"last_failure_reason,omitempty"`
}

// DefaultMaxConsecutiveFailures is how many consecutive failed polls a feed
// link survives before it is auto-disabled.
//
// It lives here rather than in the collector because two places need it and
// they cannot import each other: the job that reports failures, and the DI
// wiring that hands the threshold to the availability gateway. It is policy —
// alt-data-hub applies whatever number it is given, atomically — so it travels
// with the caller, not with the transaction (capability catalog §4-4).
const DefaultMaxConsecutiveFailures = 5

// ShouldDisable returns true if the feed has exceeded the maximum consecutive failures.
func (a *FeedLinkAvailability) ShouldDisable(maxFailures int) bool {
	return a.ConsecutiveFailures >= maxFailures
}
