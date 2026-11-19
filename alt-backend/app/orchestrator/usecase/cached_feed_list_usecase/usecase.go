package cached_feed_list_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/fetch_feed_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type CachedFeedListUsecase struct {
	legacyFeedPort fetch_feed_port.ReadAndFavoriteFeedCursorPort
	unreadFeedPort fetch_feed_port.UnreadFeedCursorPort
	allFeedPort    fetch_feed_port.FeedCursorPort
}

func NewCachedFeedListUsecase(
	legacyFeedPort fetch_feed_port.ReadAndFavoriteFeedCursorPort,
	unreadFeedPort fetch_feed_port.UnreadFeedCursorPort,
	allFeedPort fetch_feed_port.FeedCursorPort,
) *CachedFeedListUsecase {
	return &CachedFeedListUsecase{
		legacyFeedPort: legacyFeedPort,
		unreadFeedPort: unreadFeedPort,
		allFeedPort:    allFeedPort,
	}
}

const (
	// maxDailyCapRefillRounds and dailyCapRefillBatchSize bound additional DB reads
	// (~10,000 raw rows per request) when daily_cap filtering drops long runs of
	// same-day overflow rows before older visible items.
	maxDailyCapRefillRounds = 20
	dailyCapRefillBatchSize = 500
)

func (u *CachedFeedListUsecase) FetchUnreadFeedsListCursor(
	ctx context.Context,
	cursor *time.Time,
	limit int,
	excludeFeedLinkIDs []uuid.UUID,
) ([]*domain.FeedItem, bool, error) {
	if err := validateLimit(limit); err != nil {
		return nil, false, err
	}

	// Round 0 fetches limit+1 for the uncapped fast path without extra DB rows;
	// subsequent refill rounds fetch fixed 500-row batches to fill the requested page.
	visible := make([]*domain.FeedItem, 0, limit+1)
	curTime := cursor
	var curID *uuid.UUID
	rawRowsScanned := 0
	budgetExhausted := false

	for round := 0; round <= maxDailyCapRefillRounds; round++ {
		batchSize := limit + 1
		if round > 0 {
			batchSize = dailyCapRefillBatchSize
		}

		var (
			raw []*domain.FeedItem
			err error
		)
		if round == 0 || curID == nil || *curID == uuid.Nil {
			raw, err = u.unreadFeedPort.FetchUnreadFeedsListCursor(ctx, curTime, batchSize, excludeFeedLinkIDs)
		} else {
			raw, err = u.unreadFeedPort.FetchUnreadFeedsListCursorWithID(ctx, curTime, curID, batchSize, excludeFeedLinkIDs)
		}
		if err != nil {
			return nil, false, err
		}
		rawRowsScanned += len(raw)

		for _, item := range raw {
			exceeds, err := item.ExceedsDailyCap()
			if err != nil {
				return nil, false, err
			}
			if !exceeds {
				visible = append(visible, item)
				if len(visible) > limit {
					return visible[:limit], true, nil
				}
			}
		}

		if len(raw) < batchSize {
			return visible, false, nil
		}
		if round == maxDailyCapRefillRounds {
			budgetExhausted = true
			break
		}

		lastRaw := raw[len(raw)-1]
		nextTime := lastRaw.PublishedParsed
		nextID := lastRaw.FeedID
		if curTime != nil && nextTime.Equal(*curTime) && ((curID == nil && nextID == uuid.Nil) || (curID != nil && *curID == nextID)) {
			break
		}
		curTime = &nextTime
		if nextID != uuid.Nil {
			curID = &nextID
		} else {
			curID = nil
		}
	}

	if budgetExhausted {
		// External cursor is timestamp-only derived from the last visible item, so
		// returning hasMore=false avoids a client retry loop while surfacing exhaustion.
		logger.SafeWarnContext(ctx, "unread_list.daily_cap_refill_exhausted",
			"raw_rows_scanned", rawRowsScanned,
			"visible_rows_collected", len(visible),
			"requested_limit", limit,
		)
	}

	return visible, false, nil
}

func (u *CachedFeedListUsecase) FetchAllFeedsListCursor(
	ctx context.Context,
	cursor *time.Time,
	limit int,
	excludeFeedLinkIDs []uuid.UUID,
) ([]*domain.FeedItem, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}

	// Delegate to efficient single-SQL-query port.
	// The SQL query includes LEFT JOIN with read_status to set IsRead.
	return u.allFeedPort.FetchFeedsListCursor(ctx, cursor, limit, excludeFeedLinkIDs)
}

func (u *CachedFeedListUsecase) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return u.legacyFeedPort.FetchReadFeedsListCursor(ctx, cursor, limit)
}

func (u *CachedFeedListUsecase) FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return u.legacyFeedPort.FetchFavoriteFeedsListCursor(ctx, cursor, limit)
}

func validateLimit(limit int) error {
	if limit <= 0 {
		return errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		return errors.New("limit cannot exceed 100")
	}
	return nil
}
