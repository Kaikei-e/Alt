package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"alt/port/fetch_tag_cloud_port"
	"alt/utils/logger"
	"context"
	"errors"
)

// FetchTagCloudUsecase orchestrates fetching tag cloud data.
type FetchTagCloudUsecase struct {
	fetchTagCloudPort fetch_tag_cloud_port.FetchTagCloudPort
}

// NewFetchTagCloudUsecase creates a new FetchTagCloudUsecase.
func NewFetchTagCloudUsecase(port fetch_tag_cloud_port.FetchTagCloudPort) *FetchTagCloudUsecase {
	return &FetchTagCloudUsecase{fetchTagCloudPort: port}
}

// Execute fetches tag cloud data with validation.
func (u *FetchTagCloudUsecase) Execute(ctx context.Context, limit int) ([]*domain.TagCloudItem, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 500", "limit", limit)
		return nil, errors.New("limit cannot exceed 500")
	}

	logger.Logger.InfoContext(ctx, "fetching tag cloud", "limit", limit)

	items, err := u.fetchTagCloudPort.FetchTagCloud(ctx, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch tag cloud", "error", err)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched tag cloud", "count", len(items))

	// Compute 3D layout using force-directed graph
	if len(items) > 0 {
		tagNames := make([]string, len(items))
		for i, item := range items {
			tagNames[i] = item.TagName
		}

		cooccurrences, err := u.fetchTagCloudPort.FetchTagCooccurrences(ctx, tagNames)
		if err != nil {
			logger.Logger.WarnContext(ctx, "failed to fetch cooccurrences, using layout without edges", "error", err)
			cooccurrences = nil
		}

		ComputeLayout(items, cooccurrences)
	}

	return items, nil
}
