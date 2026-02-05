package fetch_random_subscription_usecase

import (
	"alt/domain"
	"alt/port/fetch_random_subscription_port"
	"alt/utils/logger"
	"context"
	"errors"
)

// ErrNoSubscriptions is an alias for domain.ErrNoSubscriptions for backwards compatibility.
// Deprecated: Use domain.ErrNoSubscriptions directly.
var ErrNoSubscriptions = domain.ErrNoSubscriptions

// FetchRandomSubscriptionUsecase handles fetching a random feed.
type FetchRandomSubscriptionUsecase struct {
	port fetch_random_subscription_port.FetchRandomSubscriptionPort
}

// NewFetchRandomSubscriptionUsecase creates a new usecase instance.
func NewFetchRandomSubscriptionUsecase(port fetch_random_subscription_port.FetchRandomSubscriptionPort) *FetchRandomSubscriptionUsecase {
	return &FetchRandomSubscriptionUsecase{
		port: port,
	}
}

// Execute fetches a random feed from the feeds table.
// Returns a Feed with title, description, and link for the Tag Trail feature.
func (u *FetchRandomSubscriptionUsecase) Execute(ctx context.Context) (*domain.Feed, error) {
	logger.Logger.InfoContext(ctx, "fetching random feed")

	feed, err := u.port.FetchRandomSubscription(ctx)
	if err != nil {
		if errors.Is(err, ErrNoSubscriptions) {
			logger.Logger.WarnContext(ctx, "no feeds found")
			return nil, err
		}
		logger.Logger.ErrorContext(ctx, "failed to fetch random feed", "error", err)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched random feed", "feedID", feed.ID, "title", feed.Title)
	return feed, nil
}
