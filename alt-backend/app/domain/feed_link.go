package domain

import "github.com/google/uuid"

// FeedLink represents a registered RSS source that can be managed by the user.
type FeedLink struct {
	ID  uuid.UUID `json:"id"`
	URL string    `json:"url"`
}

// HealthStatus represents the health classification of a feed link.
type HealthStatus string

const (
	HealthStatusHealthy  HealthStatus = "healthy"
	HealthStatusWarning  HealthStatus = "warning"
	HealthStatusError    HealthStatus = "error"
	HealthStatusInactive HealthStatus = "inactive"
	HealthStatusUnknown  HealthStatus = "unknown"
)

// FeedLinkWithHealth bundles a FeedLink with its availability data.
type FeedLinkWithHealth struct {
	FeedLink
	Availability *FeedLinkAvailability `json:"availability,omitempty"`
}

// GetHealthStatus classifies the feed health based on availability data.
func (h *FeedLinkWithHealth) GetHealthStatus() HealthStatus {
	if h.Availability == nil {
		return HealthStatusUnknown
	}
	if !h.Availability.IsActive {
		return HealthStatusInactive
	}
	if h.Availability.ConsecutiveFailures == 0 {
		return HealthStatusHealthy
	}
	if h.Availability.ConsecutiveFailures <= 2 {
		return HealthStatusWarning
	}
	return HealthStatusError
}
