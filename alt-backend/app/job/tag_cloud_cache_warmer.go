package job

import (
	"alt/usecase/fetch_tag_cloud_usecase"
	"context"
	"fmt"
	"log/slog"
)

// tagCloudExecutor abstracts the tag cloud usecase for testability.
type tagCloudExecutor interface {
	Execute(ctx context.Context, limit int) (any, error)
}

// tagCloudUsecaseAdapter wraps FetchTagCloudUsecase to satisfy tagCloudExecutor.
type tagCloudUsecaseAdapter struct {
	usecase *fetch_tag_cloud_usecase.FetchTagCloudUsecase
}

func (a *tagCloudUsecaseAdapter) Execute(ctx context.Context, limit int) (any, error) {
	return a.usecase.Execute(ctx, limit)
}

// TagCloudCacheWarmerJob returns a function suitable for the JobScheduler that
// pre-warms the tag cloud cache by executing the usecase with limit=300.
func TagCloudCacheWarmerJob(usecase *fetch_tag_cloud_usecase.FetchTagCloudUsecase) func(ctx context.Context) error {
	if usecase == nil {
		return func(ctx context.Context) error {
			slog.InfoContext(ctx, "tag cloud cache warmer skipped: usecase not configured")
			return nil
		}
	}

	return tagCloudCacheWarmerJobFn(&tagCloudUsecaseAdapter{usecase: usecase})
}

// tagCloudCacheWarmerJobFn is the testable core of the warmer job.
func tagCloudCacheWarmerJobFn(executor tagCloudExecutor) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		_, err := executor.Execute(ctx, 300)
		if err != nil {
			return fmt.Errorf("tag cloud cache warm: %w", err)
		}
		slog.InfoContext(ctx, "tag cloud cache warmed", "limit", 300)
		return nil
	}
}
