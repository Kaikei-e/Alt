package feed_inflow_stats_usecase

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	"alt/orchestrator/port/feed_inflow_port"

	"github.com/google/uuid"
)

// FeedInflowStatsUsecase aggregates 30-day per-feed item inflow and reading
// attribution for the calling user.
type FeedInflowStatsUsecase struct {
	feedInflow     feed_inflow_port.FeedInflowReadPort
	readingSources feed_inflow_port.ReadingSourcesWindowPort
}

func NewFeedInflowStatsUsecase(
	feedInflow feed_inflow_port.FeedInflowReadPort,
	readingSources feed_inflow_port.ReadingSourcesWindowPort,
) *FeedInflowStatsUsecase {
	if feedInflow == nil {
		panic("feed_inflow_stats_usecase: FeedInflowReadPort is required (see .claude/rules/di-wiring.md)")
	}
	if readingSources == nil {
		panic("feed_inflow_stats_usecase: ReadingSourcesWindowPort is required (see .claude/rules/di-wiring.md)")
	}
	return &FeedInflowStatsUsecase{
		feedInflow:     feedInflow,
		readingSources: readingSources,
	}
}

// Execute computes per-subscribed-feed inflow, counted reads (version >= 1),
// legacy opens (version == 0), and read rate over the rolling 30-day window ending at windowEnd.
func (u *FeedInflowStatsUsecase) Execute(ctx context.Context, windowEnd time.Time) ([]domain.FeedInflowStat, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user context required for feed inflow stats: %w", err)
	}
	if windowEnd.IsZero() {
		return nil, fmt.Errorf("window end timestamp is required")
	}

	windowStart := windowEnd.Add(-time.Duration(domain.FeedInflowWindowDays) * 24 * time.Hour)

	inflows, err := u.feedInflow.GetFeedInflowByUser(ctx, user.UserID, windowStart, windowEnd)
	if err != nil {
		return nil, fmt.Errorf("get feed inflow: %w", err)
	}

	sources, err := u.readingSources.ListReadingSourcesInWindow(ctx, user.UserID, windowStart, windowEnd)
	if err != nil {
		return nil, fmt.Errorf("list reading sources in window: %w", err)
	}

	readByFeed := make(map[uuid.UUID]int64, len(inflows))
	openedLegacyByFeed := make(map[uuid.UUID]int64, len(inflows))

	if len(sources) > 0 {
		seenArticles := make(map[uuid.UUID]domain.ReadingSourceEntry, len(sources))
		articleIDs := make([]uuid.UUID, 0, len(sources))
		for _, src := range sources {
			if existing, dup := seenArticles[src.ArticleID]; dup {
				if src.ReadRuleVersion > existing.ReadRuleVersion {
					seenArticles[src.ArticleID] = src
				}
				continue
			}
			seenArticles[src.ArticleID] = src
			articleIDs = append(articleIDs, src.ArticleID)
		}

		articleToFeedLink, err := u.feedInflow.BatchGetFeedLinkIDsByArticleIDs(ctx, user.UserID, articleIDs)
		if err != nil {
			return nil, fmt.Errorf("resolve article feed links: %w", err)
		}

		for artID, src := range seenArticles {
			feedLinkID, ok := articleToFeedLink[artID]
			if !ok {
				continue
			}
			if src.ReadRuleVersion >= 1 {
				readByFeed[feedLinkID]++
			} else {
				openedLegacyByFeed[feedLinkID]++
			}
		}
	}

	stats := make([]domain.FeedInflowStat, 0, len(inflows))
	for _, inf := range inflows {
		readCount := readByFeed[inf.FeedLinkID]
		openedLegacy := openedLegacyByFeed[inf.FeedLinkID]
		var readRate float64
		if inf.InflowCount > 0 {
			readRate = float64(readCount) / float64(inf.InflowCount)
		}
		stats = append(stats, domain.FeedInflowStat{
			FeedLinkID:        inf.FeedLinkID.String(),
			FeedURL:           inf.FeedURL,
			DailyCap:          inf.DailyCap,
			InflowCount:       inf.InflowCount,
			ReadCount:         readCount,
			OpenedCountLegacy: openedLegacy,
			ReadRate:          readRate,
		})
	}
	return stats, nil
}
