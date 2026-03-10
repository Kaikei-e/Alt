package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestFeedLinkWithHealth_GetHealthStatus(t *testing.T) {
	tests := []struct {
		name         string
		availability *FeedLinkAvailability
		expected     HealthStatus
	}{
		{
			name:         "unknown when no availability record",
			availability: nil,
			expected:     HealthStatusUnknown,
		},
		{
			name: "healthy when active with zero failures",
			availability: &FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            true,
				ConsecutiveFailures: 0,
			},
			expected: HealthStatusHealthy,
		},
		{
			name: "warning when active with 1 failure",
			availability: &FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            true,
				ConsecutiveFailures: 1,
			},
			expected: HealthStatusWarning,
		},
		{
			name: "warning when active with 2 failures",
			availability: &FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            true,
				ConsecutiveFailures: 2,
			},
			expected: HealthStatusWarning,
		},
		{
			name: "error when active with 3 failures",
			availability: &FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            true,
				ConsecutiveFailures: 3,
			},
			expected: HealthStatusError,
		},
		{
			name: "error when active with 5 failures",
			availability: &FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            true,
				ConsecutiveFailures: 5,
			},
			expected: HealthStatusError,
		},
		{
			name: "inactive when is_active is false",
			availability: &FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            false,
				ConsecutiveFailures: 10,
			},
			expected: HealthStatusInactive,
		},
		{
			name: "inactive takes precedence over failures",
			availability: &FeedLinkAvailability{
				FeedLinkID:          uuid.New(),
				IsActive:            false,
				ConsecutiveFailures: 0,
			},
			expected: HealthStatusInactive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := &FeedLinkWithHealth{
				FeedLink:     FeedLink{ID: uuid.New(), URL: "https://example.com/feed.xml"},
				Availability: tt.availability,
			}
			assert.Equal(t, tt.expected, link.GetHealthStatus())
		})
	}
}

func TestFeedLinkWithHealth_StructConstruction(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	reason := "connection timeout"

	link := &FeedLinkWithHealth{
		FeedLink: FeedLink{ID: id, URL: "https://example.com/feed.xml"},
		Availability: &FeedLinkAvailability{
			FeedLinkID:          id,
			IsActive:            true,
			ConsecutiveFailures: 2,
			LastFailureAt:       &now,
			LastFailureReason:   &reason,
		},
	}

	assert.Equal(t, id, link.ID)
	assert.Equal(t, "https://example.com/feed.xml", link.URL)
	assert.True(t, link.Availability.IsActive)
	assert.Equal(t, 2, link.Availability.ConsecutiveFailures)
	assert.Equal(t, &now, link.Availability.LastFailureAt)
	assert.Equal(t, &reason, link.Availability.LastFailureReason)
}
