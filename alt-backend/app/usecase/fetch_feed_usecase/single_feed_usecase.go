package fetch_feed_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"alt/utils/errors"
	"alt/utils/logger"
	"context"
)

type FetchSingleFeedUsecase struct {
	fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort
}

func NewFetchSingleFeedUsecase(fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort) *FetchSingleFeedUsecase {
	return &FetchSingleFeedUsecase{fetchSingleFeedPort: fetchSingleFeedPort}
}

func (u *FetchSingleFeedUsecase) Execute(ctx context.Context) (*domain.RSSFeed, error) {
	feed, err := u.fetchSingleFeedPort.FetchSingleFeed(ctx)
	if err != nil {
		// Check if it's already an AppContextError and enrich it with usecase context
		if appContextErr, ok := err.(*errors.AppContextError); ok {
			enrichedErr := errors.EnrichWithContext(
				appContextErr,
				"usecase",
				"FetchSingleFeedUsecase",
				"Execute",
				map[string]interface{}{
					"usecase_operation": "execute_fetch_single_feed",
				},
			)
			logger.GlobalContext.LogError(ctx, "fetch_single_feed_usecase", enrichedErr)
			return nil, enrichedErr
		}

		// Handle legacy AppError (for backward compatibility)
		if appErr, ok := err.(*errors.AppError); ok {
			// Convert legacy AppError to AppContextError
			enrichedErr := errors.NewAppContextError(
				string(appErr.Code),
				appErr.Message,
				"usecase",
				"FetchSingleFeedUsecase",
				"Execute",
				appErr.Cause,
				map[string]interface{}{
					"usecase_operation": "execute_fetch_single_feed",
					"legacy_context":    appErr.Context,
				},
			)
			logger.GlobalContext.LogError(ctx, "fetch_single_feed_usecase", enrichedErr)
			return nil, enrichedErr
		}

		// Wrap unknown errors
		unknownErr := errors.NewUnknownContextError(
			"usecase execution failed",
			"usecase",
			"FetchSingleFeedUsecase",
			"Execute",
			err,
			map[string]interface{}{
				"usecase_operation": "execute_fetch_single_feed",
			},
		)
		logger.GlobalContext.LogError(ctx, "fetch_single_feed_usecase", unknownErr)
		return nil, unknownErr
	}

	logger.GlobalContext.WithContext(ctx).Info("successfully executed fetch single feed usecase",
		"usecase", "FetchSingleFeedUsecase",
		"feed_title", feed.Title,
		"feed_items", len(feed.Items),
	)

	return feed, nil
}
