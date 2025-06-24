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
		// Wrap gateway errors with usecase context
		if appErr, ok := err.(*errors.AppError); ok {
			// Add usecase context to existing AppError
			if appErr.Context == nil {
				appErr.Context = make(map[string]interface{})
			}
			appErr.Context["usecase"] = "FetchSingleFeedUsecase"
			appErr.Context["operation"] = "Execute"
			
			logger.GlobalContext.LogError(ctx, "fetch_single_feed_usecase", appErr)
			return nil, appErr
		}
		
		// Wrap unknown errors
		unknownErr := errors.UnknownError("usecase execution failed", err, map[string]interface{}{
			"usecase":   "FetchSingleFeedUsecase",
			"operation": "Execute",
		})
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
