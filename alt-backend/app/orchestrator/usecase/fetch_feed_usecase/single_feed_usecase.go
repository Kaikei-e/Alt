package fetch_feed_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/fetch_feed_port"
	"alt/utils/errors"
	"alt/utils/logger"
	"context"
	stderrors "errors"
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
		appContextErr := &errors.AppContextError{}
		if stderrors.As(err, &appContextErr) {
			enrichedErr := errors.EnrichWithContext(
				appContextErr,
				"usecase",
				"FetchSingleFeedUsecase",
				"Execute",
				map[string]interface{}{
					"usecase_operation": "execute_fetch_single_feed",
				},
			)
			logger.SafeLogErrorWithAppContext(ctx, "fetch_single_feed_usecase", enrichedErr)
			return nil, enrichedErr
		}

		// Handle legacy AppError (for backward compatibility)
		appErr := &errors.AppError{}
		if stderrors.As(err, &appErr) {
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
			logger.SafeLogErrorWithAppContext(ctx, "fetch_single_feed_usecase", enrichedErr)
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

	logger.SafeLogInfo(ctx, "successfully executed fetch single feed usecase",
		"usecase", "FetchSingleFeedUsecase",
		"feed_title", feed.Title,
		"feed_items", len(feed.Items),
	)

	return feed, nil
}
