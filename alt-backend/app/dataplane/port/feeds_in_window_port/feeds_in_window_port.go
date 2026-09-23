package feeds_in_window_port

import (
	"alt/domain"
	"context"
)

// FeedsInWindowPort exposes the persistence boundary for fetching RSS feeds in a time window.
type FeedsInWindowPort interface {
	FetchFeedsInWindow(ctx context.Context, query domain.FeedsInWindowQuery) (*domain.FeedsInWindowPage, error)
}
