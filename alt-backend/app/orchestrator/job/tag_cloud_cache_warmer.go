package job

import (
	"alt/shared/usecase/fetch_tag_cloud_usecase"
	"context"
	"fmt"
	"log/slog"
)

// tagCloudRefresher abstracts the tag cloud usecase for testability.
type tagCloudRefresher interface {
	Refresh(ctx context.Context, limit int) (any, error)
}

// tagCloudUsecaseAdapter wraps FetchTagCloudUsecase to satisfy tagCloudRefresher.
type tagCloudUsecaseAdapter struct {
	usecase *fetch_tag_cloud_usecase.FetchTagCloudUsecase
}

func (a *tagCloudUsecaseAdapter) Refresh(ctx context.Context, limit int) (any, error) {
	return a.usecase.Refresh(ctx, limit)
}

// TagCloudCacheWarmerJob returns a function suitable for the JobScheduler that
// pre-warms the tag cloud cache by always recomputing with limit=300.
//
// FetchTagCloudUsecase is constructed unconditionally in di/article_module.go
// — unlike ImageProxyUsecase, there is no feature flag that legitimately
// leaves it nil. A nil usecase here can only be a DI wiring bug, so it must
// panic at construction time (rule 8 / .claude/rules/di-wiring.md) instead of
// silently no-op'ing on every scheduled tick forever.
func TagCloudCacheWarmerJob(usecase *fetch_tag_cloud_usecase.FetchTagCloudUsecase) func(ctx context.Context) error {
	if usecase == nil {
		panic("tag-cloud-cache-warmer: FetchTagCloudUsecase is nil — must be wired unconditionally at composition root (see .claude/rules/di-wiring.md)")
	}

	return tagCloudCacheWarmerJobFn(&tagCloudUsecaseAdapter{usecase: usecase})
}

// tagCloudCacheWarmerJobFn is the testable core of the warmer job.
func tagCloudCacheWarmerJobFn(refresher tagCloudRefresher) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		_, err := refresher.Refresh(ctx, 300)
		if err != nil {
			return fmt.Errorf("tag cloud cache warm: %w", err)
		}
		slog.InfoContext(ctx, "tag cloud cache warmed", "limit", 300)
		return nil
	}
}
