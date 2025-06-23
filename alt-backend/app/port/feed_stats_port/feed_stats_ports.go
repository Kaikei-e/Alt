package feed_stats_port

import (
	"context"
)

type FeedAmountPort interface {
	Execute(ctx context.Context) (int, error)
}

type UnsummarizedArticlesCountPort interface {
	Execute(ctx context.Context) (int, error)
}
