package domain

import (
	"time"
)

// FeedsInWindowQuery captures the filters supported by the feeds in window endpoint.
type FeedsInWindowQuery struct {
	From     time.Time
	To       time.Time
	Page     int
	PageSize int
}

// FeedsInWindowPage bundles the paginated result set returned from storage.
type FeedsInWindowPage struct {
	Total    int
	Page     int
	PageSize int
	HasMore  bool
	Feeds    []FeedRow
}
