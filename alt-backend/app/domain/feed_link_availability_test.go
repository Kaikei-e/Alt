package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFeedLinkAvailability_ShouldDisable(t *testing.T) {
	now := time.Now()
	reason := "test error"

	tests := []struct {
		name        string
		availability FeedLinkAvailability
		maxFailures int
		want        bool
	}{
		{
			name: "returns false when failures below threshold",
			availability: FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            true,
				ConsecutiveFailures: 3,
				LastFailureAt:       &now,
				LastFailureReason:   &reason,
			},
			maxFailures: 5,
			want:        false,
		},
		{
			name: "returns true when failures at threshold",
			availability: FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            true,
				ConsecutiveFailures: 5,
				LastFailureAt:       &now,
				LastFailureReason:   &reason,
			},
			maxFailures: 5,
			want:        true,
		},
		{
			name: "returns true when failures above threshold",
			availability: FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            true,
				ConsecutiveFailures: 10,
				LastFailureAt:       &now,
				LastFailureReason:   &reason,
			},
			maxFailures: 5,
			want:        true,
		},
		{
			name: "returns false when no failures",
			availability: FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            true,
				ConsecutiveFailures: 0,
			},
			maxFailures: 5,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.availability.ShouldDisable(tt.maxFailures); got != tt.want {
				t.Errorf("ShouldDisable() = %v, want %v", got, tt.want)
			}
		})
	}
}
