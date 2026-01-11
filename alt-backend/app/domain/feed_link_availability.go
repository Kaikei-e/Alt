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

// ShouldDisable returns true if the feed has exceeded the maximum consecutive failures.
func (a *FeedLinkAvailability) ShouldDisable(maxFailures int) bool {
	return a.ConsecutiveFailures >= maxFailures
}
