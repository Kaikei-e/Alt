package feed_stats_port

import (
	"context"
	"time"
)

type FeedAmountPort interface {
	Execute(ctx context.Context) (int, error)
}

type UnsummarizedArticlesCountPort interface {
	Execute(ctx context.Context) (int, error)
}

type SummarizedArticlesCountPort interface {
	Execute(ctx context.Context) (int, error)
}

type TotalArticlesCountPort interface {
	Execute(ctx context.Context) (int, error)
}

type TodayUnreadArticlesCountPort interface {
	Execute(ctx context.Context, since time.Time) (int, error)
}
